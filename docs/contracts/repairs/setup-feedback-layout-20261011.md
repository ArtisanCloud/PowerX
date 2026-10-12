# Setup 连接测试反馈与输入布局修复

日期：2026-10-11。范围：Core Web Admin；不修改 Setup 后端、Redis 服务、数据库或部署配置。

## 问题与修改

- 数据库 / Redis 测试失败原先仅显示小号灰字，重试会先清空提示。改为持久的红色 UAlert 和错误通知；测试期间显示状态，保留并标明上次结果，禁止重复点击。响应提供的请求或 Trace 标识继续显示。
- Redis RESP `NOAUTH`、`WRONGPASS`、`NOPERM` 由协议错误码映射为本地化操作提示；普通网络或数据库错误保留实际诊断。没有把认证失败改成匿名成功，也不为 Redis 设置或替换密码。
- 配置输入使用可用表单宽度；域名及证书邮箱取消多余的前置图标留白，关闭自动大写和拼写检查，保留完整原值及 title。双列配置在窄屏改为单列。
- Setup 的 USelect / URadioGroup 使用当前 Nuxt UI 的 `items` 属性，使配置选项正常呈现。

## 验证

- 9 项单元测试通过：RESP 认证错误、ofetch 响应、诊断标识、异常对象、失败 / 重试 / 成功显示。
- 2 项真实 Chromium 浏览器测试通过。对当前 3030 页面仅模拟 Setup API，不调用真实安装或修改真实配置。
- Redis 测试模拟 HTTP 400 / NOAUTH：明确提示需要密码，错误区持续显示请求标识；重试期间按钮禁用、保留上次结果，返回成功后显示成功状态。
- 测试按钮没有触发 Document 导航请求；这项证据限于本次当前页面复验，不替代历史远程现象分析。
- 域名 `powerx-dev.artisan-cloud.com`、邮箱 `administrator@artisan-cloud.com` 的输入框铺满可用宽度，左内边距小于 20 CSS px，完整值没有被输入框宽度截断。
- SFC 模板编译、四语 JSON 与 `git diff --check` 通过。浏览器测试进程已退出；用户的 3030 / 8077 服务未由 Agent 重启。

应用：本地 Nuxt dev 刷新页面即可；远程环境部署包含此修改的 Web Admin 制品后生效。Redis 启用密码认证时，仍须填写该 Redis 服务的实际部署密码。

代码：[Setup 页面](../../../web-admin/app/pages/setup/index.vue)、[反馈组件](../../../web-admin/app/components/setup/ConnectionFeedback.vue)、[协议错误映射](../../../web-admin/app/utils/setup-connection-test.ts)。

测试：[单元测试](../../../web-admin/tests/unit/setup/connection-feedback.spec.ts)、[浏览器测试](../../../web-admin/tests/e2e/setup/connection-feedback.spec.ts)。
