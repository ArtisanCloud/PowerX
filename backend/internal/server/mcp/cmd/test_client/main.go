package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// MCPRequest MCP请求结构
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse MCP响应结构
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func main() {
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "🔌 CoreX MCP 客户端测试工具")
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "========================================")

	// 启动MCP服务器进程
	cmd := exec.Command("go", "run", "./mcp/cmd/main.go")
	cmd.Dir = "/private/var/www/html/ArtisanCloud/X/PowerX/CoreX"

	// 获取stdin和stdout管道
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fatalf("创建stdin管道失败: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fatalf("创建stdout管道失败: %v", err)
	}

	// 启动服务器
	if err := cmd.Start(); err != nil {
		fatalf("启动MCP服务器失败: %v", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "✅ MCP服务器已启动")

	// 等待服务器初始化完成
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "服务器输出: %s", line)
		if strings.Contains(line, "等待客户端连接") {
			break
		}
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "🔍 开始测试MCP协议通信...")

	// 测试1: 获取工具列表
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "📋 测试1: 获取工具列表")
	testListTools(stdin, stdout)

	// 测试2: 调用list_blueprints工具
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "🔧 测试2: 调用list_blueprints工具")
	testCallTool(stdin, stdout, "list_blueprints", map[string]interface{}{})

	// 清理
	stdin.Close()
	cmd.Process.Kill()
	cmd.Wait()

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "✅ 测试完成")
}

func testListTools(stdin io.WriteCloser, stdout io.ReadCloser) {
	// 首先发送初始化请求
	initReq := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	sendRequest(stdin, initReq)
	initResponse := readResponse(stdout)

	if initResponse.Error != nil {
		logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "❌ 初始化失败: %v", initResponse.Error)
		return
	} else {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "✅ 初始化成功: %v", initResponse.Result)
	}

	// 然后获取工具列表
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
		Params: map[string]interface{}{
			"sessionId": "test-session-123",
		},
	}

	sendRequest(stdin, req)
	response := readResponse(stdout)

	if response.Error != nil {
		logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "❌ 错误: %v", response.Error)
	} else {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "✅ 成功获取工具列表: %v", response.Result)
	}
}

func testCallTool(stdin io.WriteCloser, stdout io.ReadCloser, toolName string, params map[string]interface{}) {
	// 添加sessionId到参数中
	params["sessionId"] = "test-session-123"

	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      toolName,
			"arguments": params,
		},
	}

	sendRequest(stdin, req)
	response := readResponse(stdout)

	if response.Error != nil {
		logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "❌ 调用工具失败: %v", response.Error)
	} else {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "✅ 工具调用成功: %v", response.Result)
	}
}

func sendRequest(stdin io.WriteCloser, req MCPRequest) {
	data, err := json.Marshal(req)
	if err != nil {
		fatalf("序列化请求失败: %v", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "📤 发送请求: %s", string(data))

	_, err = stdin.Write(append(data, '\n'))
	if err != nil {
		fatalf("发送请求失败: %v", err)
	}
}

func readResponse(stdout io.ReadCloser) MCPResponse {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()

		// 跳过日志行，只处理JSON响应
		if strings.HasPrefix(line, "{") {
			var response MCPResponse
			if err := json.Unmarshal([]byte(line), &response); err == nil {
				logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), "📥 收到响应: %s", line)
				return response
			}
		}
	}

	return MCPResponse{}
}

func fatalf(format string, args ...any) {
	logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "mcp.test_client"}), format, args...)
	os.Exit(1)
}
