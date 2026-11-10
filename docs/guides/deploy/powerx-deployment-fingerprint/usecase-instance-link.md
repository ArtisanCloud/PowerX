# 心跳与挑战核验

1. **配置入口**：源码默认修改 `backend/etc/config.yaml`；容器修改实际挂载的 config.yaml，systemd 修改实际 `POWERX_CONFIG` 文件。合并以下配置，保留原市场地址：

   ```yaml
   plugin:
     marketplace:
       public_site_url: https://admin.example.com/console/
   ```

   地址是网站基址，不是对象存储地址、Core 监听地址或 API root。HTTP 可以声明但不能完成实例关联。无效值会记录警告并暂停扩展，普通登记仍可继续。

2. **加载配置**：在原服务终端按原流程重启后端；网站上游通过服务端 `POWERX_BACKEND` 指向固定 Core，例如 `http://127.0.0.1:8077/api/v1`。预期原环境私有实例 ID 和凭证继续复用。失败看市场实例接入状态和后端日志，不删除私有身份文件。Core 没有新增自动热加载器，配置文件不会自行生效。
3. **环境覆盖**：可以显式设置 `POWERX_MARKETPLACE_PUBLIC_SITE_URL`；设置空字符串表示清除，未设置时保留配置文件值。默认 Compose 不自动注入一个空值覆盖文件；需要环境覆盖时在服务配置中显式添加该变量。
4. **创建挑战**：Marketplace 管理员访问 `/admin/distribution` →「部署识别」，选择与当前入口相同的已登记实例，创建挑战。等待既有心跳（默认 300 秒，可使用原 `heartbeat_seconds` 配置）领取，手动刷新后核验。预期显示关联结果及具体时间；未领取或过期则重新挑战，不新增页面轮询。
5. **只读证明检查**：`curl -i 'https://admin.example.com/console/.well-known/powerx-instance.json?challenge_id=<市场创建的UUID>'`。有效时返回 `no-store`，裸 JSON 只有 schema、challenge_id、nonce、expires_at；未知/过期返回 404，维护或 Core 不可达返回 503。Core 直连为 `<api_prefix>/system/marketplace-proof`，采用 `code=200,data=...` envelope；不需要也不要在公开请求中添加实例令牌。
6. **替换/清空**：修改为新入口或 `public_site_url: ""` 并加载后，本地旧证明立即停止；下一次成功扩展心跳同步市场。预期旧挑战和当前关联失效而历史保留；改回旧入口须重新核验。其他实例不应受影响。

本地测试命令（根目录）为 `cd backend && go test -race ./internal/service/plugin_marketplace ./internal/infra/plugin/marketplace -run '^TestDeployment' -count=1`。测试使用隔离 HTTP 和私有临时状态，不代表生产市场允许回环/内网抓取。真实验收需两套独立 PowerX、有效 HTTPS 和相同正式发行包，记录实例归属、重启、过期和在线市场回退恢复结果。
