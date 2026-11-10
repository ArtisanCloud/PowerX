# 发行清单与公开标识

1. **生成发行产物**：根目录按原流程执行 `make dist-systemd`。预期 `dist/systemd/<版本>/backend/powerx.build.json` 对应同目录最终主程序，网站 `.output/public/.well-known/powerx.json` 对应最终 JS/CSS。失败时查构建输出，不从旧包拷贝侧车。
2. **Docker 构建**：两种 Dockerfile 使用根目录 build context；共同传入 `POWERX_VERSION`、`POWERX_SOURCE_SHA` 和 UTC `POWERX_BUILD_TIME`。后端最终路径为 `/app/powerx-app.build.json`；前端清单随 `.output` 发布。子路径部署额外传入前端 `NUXT_APP_BASE_URL=/console/`，网关需将该子路径保留并转发到 Nitro。镜像实际构建和公网代理需部署环境验证。
3. **本地先验证后端**：使用已有服务访问 `curl -sS http://127.0.0.1:8077/api/v1/system/product`。API prefix 取实际 `server.api_prefix`，不能写死。源码 `make dev` 没有发行侧车时，启动自动计算当前实际可执行程序的构建摘要，响应保持 `state=development` 并包含 `build_info`；相同程序摘要稳定，程序重新构建后可能变化。正式包自检成功为 `available`。缺失正式侧车、侧车损坏或摘要不符仍保持失败状态，不能借开发回退隐藏。404 先核对运行的进程是否加载新代码及 prefix。
4. **网站检查**：访问 `https://<网站>/<可选子路径>/.well-known/powerx.json`，预期 JSON、`Cache-Control: no-cache`；HTML 含 `generator=PowerX`。开发模式未生成发行清单时清单可能不可用，不能当作官方构建。
5. **实际构建输出检查**：在 `web-admin` 执行 `npm run build`，再执行 `npm run fingerprint:check`。预期根路径和子路径都通过，检查脚本记录并清理自己创建的 Nitro/上游进程。可用 `node scripts/check-deployment-fingerprint-output.mjs <隔离output目录>` 检查隔离输出；失败看首个 HTTP/摘要断言。
6. **自动上报查看**：Marketplace 管理员进入 `/admin/distribution` →「实例与指纹」→「刷新状态」。源码启动与正式包都自动随登记、心跳上报后端构建指纹，无需使用方上传文件或配置公网域名。列表显示指纹摘要与「实例上报」，开发模式另标「开发构建」；详情「构建指纹」显示完整 build_id、版本、组件及接收时间。它是实例构建声明，不等价于网站独立核验或受信任官方清单入库。实例重启继续使用原身份。
7. **网站资源匹配**：进入「发行指纹库」导入本次实际前端清单，再在「部署识别」提交公开网址并执行识别。预期区分网站声明和受信任资源匹配；失败查看本次运行的读取状态，不因图标或版本文字相同就认定关联。发行指纹入库是发布方/市场管理员职责，不需要逐个使用方提供清单。

完整二进制的快速只读自检命令为 `<binary> --product-info`，不连接业务数据库。源码回归命令（根目录）为 `cd backend && go test ./internal/productinfo ./internal/transport/http/public/product -count=1`。
