package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/types"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

// RenderPlanTool 真正把蓝图渲染成代码文件
func RenderPlanTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// 取 flow_id
	flowID, _ := args["flow_id"].(string)
	if flowID == "" {
		raw, ok := args["plan"]
		if !ok {
			return nil, fmt.Errorf("缺少 flow_id 或 plan 参数")
		}
		b, _ := json.Marshal(raw)
		var plan map[string]interface{}
		if err := json.Unmarshal(b, &plan); err != nil {
			return nil, fmt.Errorf("plan 解析失败: %w", err)
		}
		fid, ok := plan["flow_id"].(string)
		if !ok || fid == "" {
			return nil, fmt.Errorf("plan 中缺少 flow_id 字段")
		}
		flowID = fid
	}

	// 读蓝图 YAML（顶级或 usecases 子目录）
	cfg := config.GetGlobalConfig().MCP
	base := cfg.FlowSpecsConfig.Blueprints
	paths := []string{
		filepath.Join(base, flowID+".yaml"),
		filepath.Join(base, "usecases", flowID+".yaml"),
	}
	var data []byte
	var err error
	for _, p := range paths {
		data, err = ioutil.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("无法加载蓝图文件: %w", err)
	}
	// fmt2.Dump(paths)
	// fmt2.Dump("蓝图 YAML:", string(data))

	var flow schemas.Flow
	if err = yaml.Unmarshal(data, &flow); err != nil {
		return nil, fmt.Errorf("解析蓝图失败: %w", err)
	}

	// 合并 overrides.variables
	if ov, ok := args["overrides"].(map[string]interface{}); ok {
		if vars, ok2 := ov["variables"].(map[string]interface{}); ok2 {
			if flow.Variables == nil {
				flow.Variables = make(map[string]string)
			}
			for k, v := range vars {
				flow.Variables[k] = fmt.Sprint(v)
			}
		}
	}

	// … 读取完 flow、合并好 flow.Variables …
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": cases.Title(language.Und, cases.NoLower).String,
	}
	files := map[string]string{}

	for _, step := range flow.Steps {
		if step.Type != "generate" {
			continue
		}
		// 渲染路径
		//fmt2.Dump("output path:", step.Parameters["output_path"])
		pathT := template.New("p").Funcs(funcMap)
		pathT.Parse(step.Parameters["output_path"].(string))
		var pBuf strings.Builder
		pathT.Execute(&pBuf, flow.Variables)
		outPath := pBuf.String()

		// 找模板
		//fmt2.Dump("step.Action =", step.Action)
		tplName, ok := templateMap[step.Action]
		if !ok {
			// 如果找不到，跳过或打印警告
			fmt.Println("未找到模板映射:", step.Action)
			continue
		}
		tplPath := filepath.Join(cfg.TemplatesDir, tplName+".tmpl")
		//fmt2.Dump("模板路径:", tplPath)
		raw, err := ioutil.ReadFile(tplPath)
		if err != nil {
			continue
		}
		// 渲染内容
		ct := template.New("c").Funcs(funcMap)
		ct.Parse(string(raw))
		data := map[string]interface{}{
			"Vars":   flow.Variables,
			"Params": step.Parameters,
		}
		var cBuf strings.Builder
		ct.Execute(&cBuf, data)

		files[outPath] = cBuf.String()
	}

	// 保存文件到tmp/render目录
	if err := saveFilesToTmp(files, flowID); err != nil {
		logger.ErrorF(ctx, "保存文件到tmp目录失败:%v", err)
	}

	// 返回结果
	resp := map[string]interface{}{"files": files}
	bts, _ := json.Marshal(resp)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "application/json", Text: string(bts)},
		},
	}, nil
}

// saveFilesToTmp 保存文件到tmp/render目录
func saveFilesToTmp(files map[string]string, flowID string) error {
	// 创建tmp/render目录
	tmpDir := filepath.Join("temp", "render", flowID)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 保存每个文件
	for filePath, content := range files {
		// 去掉转义字符，提高可读性
		cleanContent := strings.ReplaceAll(content, "\\n", "\n")
		cleanContent = strings.ReplaceAll(cleanContent, "\\t", "\t")
		cleanContent = strings.ReplaceAll(cleanContent, "\\\\", "\\")
		cleanContent = strings.ReplaceAll(cleanContent, "\\\"", "\"")

		// 构建保存路径
		fileName := filepath.Base(filePath)
		if fileName == "." || fileName == "" {
			fileName = strings.ReplaceAll(filePath, "/", "_")
		}
		savePath := filepath.Join(tmpDir, fileName)

		// 如果文件名重复，添加序号
		if _, err := os.Stat(savePath); err == nil {
			ext := filepath.Ext(fileName)
			name := strings.TrimSuffix(fileName, ext)
			for i := 1; ; i++ {
				newName := fmt.Sprintf("%s_%d%s", name, i, ext)
				newPath := filepath.Join(tmpDir, newName)
				if _, err := os.Stat(newPath); os.IsNotExist(err) {
					savePath = newPath
					break
				}
			}
		}

		// 写入文件
		if err := ioutil.WriteFile(savePath, []byte(cleanContent), 0o644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", savePath, err)
		}
	}

	// 输出保存信息
	//fmt2.Dump(fmt.Sprintf("已保存 %d 个文件到目录: %s", len(files), tmpDir))
	return nil
}

// 在 RenderPlanTool 之上或函数最前面定义：
var templateMap = map[string]string{
	types.ActionSuffix + types.TplGenerateDomainModel:         types.TplGenerateDomainModel,
	types.ActionSuffix + types.TplGenerateGormModel:           types.TplGenerateGormModel,
	types.ActionSuffix + types.TplGenerateRepositoryInterface: types.TplGenerateRepositoryInterface,
	types.ActionSuffix + types.TplGenerateRepositoryImpl:      types.TplGenerateRepositoryImpl,
	types.ActionSuffix + types.TplGenerateMigration:           types.TplGenerateMigration,
	types.ActionSuffix + types.TplGenerateUseCase:             types.TplGenerateUseCase,
	types.ActionSuffix + types.TplGenerateDTO:                 types.TplGenerateDTO,
	types.ActionSuffix + types.TplGenerateAdapter:             types.TplGenerateAdapter,
	types.ActionSuffix + types.TplGenerateAPI:                 types.TplGenerateAPI,
	types.ActionSuffix + types.TplGenerateHandler:             types.TplGenerateHandler,
	types.ActionSuffix + types.TplGenerateGRPCService:         types.TplGenerateGRPCService,
	types.ActionSuffix + types.TplGenerateTests:               types.TplGenerateTests,
	types.ActionSuffix + types.TplGenerateMocks:               types.TplGenerateMocks,
}
