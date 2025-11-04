# WebSocket 实现指南

## 概述

本项目实现了基于同源反代 + 子协议携带 Token 的 WebSocket 连接方案，确保开发和生产环境的一致性和安全性。

## 架构设计

```
前端 (浏览器)
    ↓ wss://domain.com/api/agents/stream/ws
    ↓ Sec-WebSocket-Protocol: bearer.base64url_token
开发环境: Nitro 插件代理
生产环境: Nginx 代理
    ↓ 透传所有头部
后端 WebSocket 服务 (127.0.0.1:8077)
```

## 核心文件

### 1. Nitro WebSocket 代理插件

**文件**: `server/plugins/ws-proxy.ts`

- 处理 WebSocket 升级请求
- 透传 `Sec-WebSocket-Protocol` 头部
- 只代理 `/api/agents/stream/ws` 路径

### 2. 前端连接管理

**文件**: `app/composables/agent/useDualChannelConnection.ts`

- 统一使用同源 WebSocket URL
- Token 仅通过子协议传递
- 实现心跳、重连、取消机制

### 3. 页面集成

**文件**: `app/pages/agent/index.vue`

- 使用 `sendMessage(message, flowId, meta)` 发送消息
- 支持会话上下文传递

## 安全特性

1. **Token 保护**: 不在 URL 中暴露认证信息
2. **同源策略**: 避免跨域安全问题
3. **子协议认证**: 标准的 WebSocket 认证方式

## 连接增强

1. **指数退避重连**: 1s → 2s → 4s → 8s → 16s → 30s
2. **心跳机制**: 每 25 秒发送 ping 防止连接断开
3. **请求取消**: 支持 AbortController 取消 SSE 请求

## 部署配置

### 开发环境

无需额外配置，Nitro 插件自动处理 WebSocket 代理。

### 生产环境 (Nginx)

```nginx
location /api/agents/stream/ws {
    proxy_pass http://127.0.0.1:8077;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header Origin $http_origin;
    proxy_set_header Sec-WebSocket-Protocol $http_sec_websocket_protocol;
    proxy_read_timeout 3600;
    proxy_send_timeout 3600;
}
```

## 测试验证

### 1. 构建测试

```bash
npm run build
```

### 2. WebSocket 连接测试

```bash
node test-websocket.js
```

### 3. 手动测试 (websocat)

```bash
websocat "ws://localhost:3000/api/agents/stream/ws?probe=1" \
  --header "Sec-WebSocket-Protocol: bearer.test_token"
```

## 故障排除

### 常见问题

1. **连接被拒绝**: 检查后端服务是否在 8077 端口运行
2. **认证失败**: 确认 Token 格式正确 (`bearer.base64url_token`)
3. **代理不工作**: 检查 Nitro 插件是否正确加载

### 调试方法

1. 查看浏览器开发者工具的网络面板
2. 检查服务器日志中的 WebSocket 连接信息
3. 使用 `console.log` 在代理插件中添加调试信息

## API 使用示例

```typescript
// 发送消息
await chat.sendMessage("Hello", "chat", {
  sessionId: "session-123",
  agentId: 456,
});

// 监听消息
chat.onMessage = (data) => {
  console.log("收到消息:", data);
};

// 监听错误
chat.onError = (error) => {
  console.error("连接错误:", error);
};
```

## 性能优化

1. **连接复用**: 同一会话复用 WebSocket 连接
2. **智能重连**: 指数退避避免频繁重连
3. **心跳优化**: 25 秒间隔平衡性能和稳定性

## 未来扩展

1. **多后端支持**: 可扩展支持多个后端服务
2. **负载均衡**: 配合 Nginx upstream 实现负载均衡
3. **监控集成**: 添加连接状态监控和告警
