# Sentry 日志与链路追踪集成

> 规划 PowerX Web Admin 中的前端 Sentry 集成方案，包括错误采集、性能监控、Release 版本与后端追踪关联。

---

## 1. 初始化步骤

1. 安装依赖：
   ```bash
   npm install @sentry/vue @sentry/tracing @sentry-internal/tracing
   ```
2. 创建插件 `app/plugins/sentry.client.ts`：
   ```ts
   import * as Sentry from "@sentry/vue";

   export default defineNuxtPlugin((nuxtApp) => {
     if (!process.client || !process.env.SENTRY_DSN) return;

     const app = nuxtApp.vueApp;

     Sentry.init({
       app,
       dsn: process.env.SENTRY_DSN,
       environment: process.env.NUXT_ENV || "development",
       release: process.env.NUXT_APP_VERSION,
       integrations: [
         Sentry.browserTracingIntegration({ router: nuxtApp.$router }),
         Sentry.replayIntegration({ stickySession: true }),
       ],
       tracesSampleRate: 0.1,
       replaysSessionSampleRate: 0.05,
       replaysOnErrorSampleRate: 1.0,
     });
   });
   ```
3. 在 `.env` 中配置 `SENTRY_DSN`、`SENTRY_ENV`，并记录到 `Env_Variables_Schema.md`。

---

## 2. 错误上报策略

- **自动上报**：Sentry 会捕捉未处理异常、Unhandled Promise Rejection。  
- **手动上报**：对业务错误调用 `Sentry.captureException(err, { tags, extra })`，附加 `tenantId`、`agentId` 等上下文。  
- **过滤敏感信息**：在初始化时设置 `beforeSend`，过滤 Token、用户隐私数据。

---

## 3. 性能与前端 Trace

- 启用 `browserTracingIntegration` 后，Sentry 会记录页面加载、路由切换、Ajax 请求性能数据。  
- 设置 `tracesSampleRate`（0-1）控制采样率，生产环境推荐 0.05-0.2。  
- 对关键流程（插件安装、工作流保存）可使用 `Sentry.startSpan()` 手动包裹，精确测量耗时。

---

## 4. 与后端链路关联

- 后端（Go/Node/Python 等）需同时启用 Sentry 或 OpenTelemetry，使用相同的 `traceparent`。  
- 前端在发送请求时将 `sentry-trace`、`baggage` header 注入（`@sentry/vue` 自动完成），后端解析后创建子 Span。  
- 在 Sentry UI 可查看完整请求链路：前端操作 → API → 微服务。

---

## 5. 日志上下文

- 设置 `Sentry.setUser({ id, email, tenant })`，帮助定位用户反馈。  
- 使用 `Sentry.setTag("feature", "agent-chat")` 标记功能模块，便于筛选。  
- 在 `catch` 中附带 `Sentry.captureException(err, { contexts: { session: {...} } })`。

---

## 6. 发布与版本管理

- 提交发布前执行：
  ```bash
  export SENTRY_RELEASE=$CI_COMMIT_SHA
  sentry-cli releases new $SENTRY_RELEASE
  sentry-cli releases set-commits --auto $SENTRY_RELEASE
  sentry-cli releases finalize $SENTRY_RELEASE
  ```
- 将 `NUXT_APP_VERSION` 与 Sentry Release 对齐，在前端初始化中传入。  
- 发生错误时可直接跳转到相关 commit，便于回溯。

---

## 7. 咨询与隐私

- 遵循隐私政策，不收集敏感字段（Token、聊天内容）。  
- 可通过 `beforeSend` 将消息正文模糊化或截断。  
- 提供设置项允许管理员关闭会话录制（Sentry Replay）。

---

## 8. Review Checklist

- [ ] 所有关键页面的异常均被 Sentry 捕捉。  
- [ ] 自定义错误上报附带必要上下文，不包含敏感数据。  
- [ ] Release 版本号与部署管理一致。  
- [ ] `tracesSampleRate`/`replaysSessionSampleRate` 合理配置。  
- [ ] 文档与环境变量同步更新。

---

## 9. 后续计划

- 集成 Sentry Issues 到告警系统（Slack/飞书）。  
- 使用 Sentry Dashboards 监控前端错误趋势。  
- 引入 OpenTelemetry 跨服务聚合，形成统一可观测性平台。  
- 在 QA 环境开启全量采样，快速发现回归问题。
