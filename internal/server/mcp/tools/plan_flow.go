package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/types"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/mark3labs/mcp-go/mcp"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PlanConfig 运行时计划配置
type PlanConfig struct {
	Overrides        map[string]map[string]interface{} `json:"overrides"`
	MaxParallelSteps int                               `json:"max_parallel_steps"`
	IncludeSteps     []string                          `json:"include_steps"`
	ExcludeSteps     []string                          `json:"exclude_steps"`
	DryRun           bool                              `json:"dry_run"`
}

// mcp/tools/plan_flow.go

// PlanFlowTool 对给定 Flow 进行拓扑排序并生成执行计划
func PlanFlowTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// 获取并解析 flow 参数
	args := request.GetArguments()
	rawFlow, ok := args[types.ParamFlow]
	if !ok {
		return nil, fmt.Errorf("缺少 %s 参数", types.ParamFlow)
	}
	// 将 rawFlow 转为 JSON，再 Unmarshal 到 schema.Flow
	buf, err := json.Marshal(rawFlow)
	if err != nil {
		return nil, fmt.Errorf("flow 数据序列化失败: %w", err)
	}
	var flow schemas.Flow
	if err := json.Unmarshal(buf, &flow); err != nil {
		return nil, fmt.Errorf("flow 数据解析失败: %w", err)
	}

	// 解析运行时参数
	planConfig := parsePlanConfig(args)

	// 应用运行时覆盖参数
	if err := applyOverrides(&flow, planConfig.Overrides); err != nil {
		return nil, fmt.Errorf("应用参数覆盖失败: %w", err)
	}

	// 过滤步骤
	if err := filterSteps(&flow, planConfig.IncludeSteps, planConfig.ExcludeSteps); err != nil {
		return nil, fmt.Errorf("过滤步骤失败: %w", err)
	}

	// 构建节点映射和入度
	steps := flow.Steps
	stepMap := make(map[string]schemas.Step, len(steps))
	inDegree := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, s := range steps {
		stepMap[s.ID] = s
		inDegree[s.ID] = 0
	}
	for _, s := range steps {
		for _, nxt := range s.NextSteps {
			adj[s.ID] = append(adj[s.ID], nxt)
			inDegree[nxt]++
		}
	}

	// 拓扑排序队列
	queue := make([]string, 0, len(steps))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// 执行调度，收集步骤信息
	results := make([]map[string]interface{}, 0, len(steps))
	resultsMutex := sync.Mutex{}

	// 并发执行步骤
	for len(queue) > 0 {
		// 取出一批可并行执行的步骤
		batchSize := planConfig.MaxParallelSteps
		if batchSize > len(queue) {
			batchSize = len(queue)
		}
		batch := make([]string, batchSize)
		copy(batch, queue[:batchSize])
		queue = queue[batchSize:]

		// 并发执行当前批次的步骤
		var wg sync.WaitGroup
		for _, stepID := range batch {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				step := stepMap[id]
				result := executeStep(ctx, step, planConfig.DryRun)

				resultsMutex.Lock()
				results = append(results, result)
				resultsMutex.Unlock()
			}(stepID)
		}
		wg.Wait()

		// 更新入度，将下游步骤加入队列
		for _, stepID := range batch {
			for _, nxt := range adj[stepID] {
				inDegree[nxt]--
				if inDegree[nxt] == 0 {
					queue = append(queue, nxt)
				}
			}
		}
	}

	// 生成最终 plan
	planStatus := "completed"
	if planConfig.DryRun {
		planStatus = "preview"
	} else {
		// 检查是否有失败的步骤
		for _, result := range results {
			if status, ok := result["status"].(string); ok && status == "failed" {
				planStatus = "failed"
				break
			}
		}
	}

	plan := map[string]interface{}{
		"plan_id":     uuid.New().String(),
		"flow_id":     flow.ID,
		"name":        flow.Name,
		"description": flow.Description,
		"tasks":       results,
		"status":      planStatus,
		"created_at":  time.Now().Format(time.RFC3339),
		"dry_run":     planConfig.DryRun,
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("生成 plan JSON 失败: %w", err)
	}

	// 返回 JSON 文本
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: types.FieldText, Text: string(planJSON)},
		},
	}, nil
}

// parsePlanConfig 解析运行时计划配置参数
func parsePlanConfig(args map[string]interface{}) *PlanConfig {
	config := &PlanConfig{
		MaxParallelSteps: 1, // 默认值
		DryRun:           false,
	}

	// 解析 overrides
	if v, ok := args["overrides"].(map[string]interface{}); ok {
		config.Overrides = make(map[string]map[string]interface{})
		for stepID, stepOverrides := range v {
			if overrideMap, ok := stepOverrides.(map[string]interface{}); ok {
				config.Overrides[stepID] = overrideMap
			}
		}
	}

	// 解析 max_parallel_steps
	if v, ok := args["max_parallel_steps"].(float64); ok {
		config.MaxParallelSteps = int(v)
	}

	// 解析 include_steps
	if v, ok := args["include_steps"].([]interface{}); ok {
		for _, step := range v {
			if stepStr, ok := step.(string); ok {
				config.IncludeSteps = append(config.IncludeSteps, stepStr)
			}
		}
	}

	// 解析 exclude_steps
	if v, ok := args["exclude_steps"].([]interface{}); ok {
		for _, step := range v {
			if stepStr, ok := step.(string); ok {
				config.ExcludeSteps = append(config.ExcludeSteps, stepStr)
			}
		}
	}

	// 解析 dry_run
	if v, ok := args["dry_run"].(bool); ok {
		config.DryRun = v
	}

	return config
}

// applyOverrides 应用运行时参数覆盖
func applyOverrides(flow *schemas.Flow, overrides map[string]map[string]interface{}) error {
	if overrides == nil {
		return nil
	}

	// 创建步骤映射以便快速查找
	stepMap := make(map[string]*schemas.Step)
	for i := range flow.Steps {
		stepMap[flow.Steps[i].ID] = &flow.Steps[i]
	}

	// 应用覆盖参数
	for stepID, stepOverrides := range overrides {
		step, exists := stepMap[stepID]
		if !exists {
			continue // 跳过不存在的步骤
		}

		// 应用参数覆盖
		if params, ok := stepOverrides["parameters"].(map[string]interface{}); ok {
			if step.Parameters == nil {
				step.Parameters = make(map[string]interface{})
			}
			for key, value := range params {
				step.Parameters[key] = value
			}
		}

		// 应用超时覆盖
		if timeout, ok := stepOverrides["timeout"].(float64); ok {
			step.Timeout = int(timeout)
		}

		// 应用重试次数覆盖 (使用Retry字段)
		if retryCount, ok := stepOverrides["retry_count"].(float64); ok {
			step.Retry = int(retryCount)
		}

		// 注意：schema.Step没有Enabled字段，这里暂时跳过
		// 如果需要启用/禁用功能，可以考虑在Parameters中添加特殊标记
	}

	return nil
}

// filterSteps 根据包含和排除列表过滤步骤
func filterSteps(flow *schemas.Flow, includeSteps, excludeSteps []string) error {
	if len(includeSteps) == 0 && len(excludeSteps) == 0 {
		return nil // 无需过滤
	}

	// 创建包含和排除的映射
	includeMap := make(map[string]bool)
	excludeMap := make(map[string]bool)

	for _, stepID := range includeSteps {
		includeMap[stepID] = true
	}
	for _, stepID := range excludeSteps {
		excludeMap[stepID] = true
	}

	// 过滤步骤
	filteredSteps := make([]schemas.Step, 0, len(flow.Steps))
	for _, step := range flow.Steps {
		// 如果有包含列表，只保留在列表中的步骤
		if len(includeSteps) > 0 && !includeMap[step.ID] {
			continue
		}
		// 如果在排除列表中，跳过该步骤
		if excludeMap[step.ID] {
			continue
		}
		// 注意：schema.Step没有Enabled字段，这里暂时跳过启用状态检查
		// 如果需要启用/禁用功能，可以考虑在Parameters中添加特殊标记
		filteredSteps = append(filteredSteps, step)
	}

	flow.Steps = filteredSteps
	return nil
}

// executeStep 执行单个步骤
func executeStep(ctx context.Context, step schemas.Step, dryRun bool) map[string]interface{} {
	startTime := time.Now()
	result := map[string]interface{}{
		"id":         uuid.New().String(),
		"step_id":    step.ID,
		"name":       step.Name,
		"type":       step.Type,
		"action":     step.Action,
		"parameters": step.Parameters,
		"started_at": startTime.Format(time.RFC3339),
	}

	if dryRun {
		// 预览模式，不实际执行
		result["status"] = "preview"
		result["result"] = "This step would be executed in non-dry-run mode"
		result["duration"] = 0
		return result
	}

	// TODO: 这里需要根据step.Action查找并调用对应的工具处理器
	// 由于存在导入循环问题，暂时使用模拟执行
	// 在实际实现中，应该通过依赖注入或其他方式获取注册表

	// 模拟执行时间
	time.Sleep(10 * time.Millisecond)

	endTime := time.Now()
	duration := endTime.Sub(startTime).Milliseconds()

	// 模拟执行结果
	switch step.Action {
	case "hash_password":
		result["status"] = "completed"
		result["result"] = map[string]interface{}{
			"hashed_password": "$2a$10$...", // 模拟哈希结果
			"algorithm":       "bcrypt",
		}
	case "validate_email":
		result["status"] = "completed"
		result["result"] = map[string]interface{}{
			"is_valid": true,
			"domain":   "example.com",
		}
	case "create_user":
		result["status"] = "completed"
		result["result"] = map[string]interface{}{
			"user_id": uuid.New().String(),
			"created": true,
		}
	default:
		// 未知操作，标记为跳过
		result["status"] = "skipped"
		result["result"] = fmt.Sprintf("Unknown action: %s", step.Action)
	}

	result["completed_at"] = endTime.Format(time.RFC3339)
	result["duration"] = duration

	return result
}
