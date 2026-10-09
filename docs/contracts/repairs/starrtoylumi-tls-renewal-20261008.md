# 2026-10-08 Starrtoylumi 双域名自动续期核对

## 结果与验收边界

已在阿里云服务器 `47.116.10.200` 补齐自动续期后的 Nginx 重载，验证共享证书的 HTTP-01 续期演练和重载成功，并停用重复的旧版定时器。自动任务已配置，但演练期间出现过海外验证节点 TCP/80 连接超时，云端安全组及海外访问限制尚未核对，不能据此宣称长期网络稳定性已验收。

机器当前运行宿主机 Nginx 和 Snap Certbot，本次没有迁移到 XDocker、安装第二套服务或修改 PowerX 应用配置。证据：[结构化远程和公网核对结果](../evidence/starrtoylumi-tls-renewal-20261008.json)。

## 当前正式证书

| 项目 | 核实值 |
| --- | --- |
| 域名 | `agent.starrtoylumi.com`、`agent-dev.starrtoylumi.com` |
| DNS | 两者均解析到 `47.116.10.200` |
| Certbot lineage | `starrtoylumi-agent`，一张 SAN 证书覆盖两个域名 |
| Nginx 使用路径 | `/etc/letsencrypt/live/starrtoylumi-agent/fullchain.pem` |
| 到期时间 | `2026-11-15 06:19:40 UTC`，即北京时间 `14:19:40` |
| SHA256 | `3baabfc36664f2c1d90a81b1a3baf984ae848decfa837100ae970cabd9763d02` |
| 验证方式 | `webroot` / HTTP-01，目录 `/var/www/letsencrypt` |
| 公网验收 | 两个域名 TLS 校验通过，主页及 `/api/v1/health` 均返回 200 |

正式证书的序列号和 SHA256 在操作前后相同；没有强制续签或替换为测试证书。

## 实际操作

1. 备份共享证书 renewal 配置、原定时器定义与启用状态、公开证书指纹至服务器 `/root/powerx-tls-backups/20261008-starrtoylumi/`（目录权限 0700）。备份未复制到 Git。
2. 使用现有 Snap Certbot `5.8.0` 的正式配置命令保存 deploy hook：

   ```bash
   certbot reconfigure --cert-name starrtoylumi-agent \
     --deploy-hook '/usr/sbin/nginx -t && /bin/systemctl reload nginx' \
     --run-deploy-hooks --non-interactive
   ```

   `renew_hook` 已写入 `/etc/letsencrypt/renewal/starrtoylumi-agent.conf`。成功演练于北京时间 `19:25:52` 调用 Nginx 重载，退出码 0，master PID `3446430` 保持不变。
3. 普通 `certbot renew --non-interactive --no-random-sleep-on-renew` 返回 0，现有三张证书均未到续期窗口，因此没有申请正式证书。
4. 确认 `snap.certbot.renew.timer` 已 `enabled/active` 后，停用重复的 apt `certbot.timer`。保留的 Snap 定时器每天北京时间 `05:04` 和 `15:29` 运行，核对时下一次为 `2026-10-09 05:04`；systemd 在开机时启用该定时器。每日检查会按 Certbot 的续期条件决定是否申请证书，日期并不意味着每日更换证书。
5. 清除两项服务的历史 failed 标记；原失败原因仍保存在 journal 中。重新检查公网 TLS、网页与健康接口均正常。

Certbot 的 `--run-deploy-hooks` 在成功演练后使用当前有效证书执行 hook，测试证书不会成为正式证书。[Certbot 官方文档](https://eff-certbot.readthedocs.io/en/stable/using.html#renewing-certificates)

## 尚待核对的网络问题

- 北京时间 `19:22:51`，两个域名的首次续期演练成功。
- 随后一次配置演练失败：`agent-dev.starrtoylumi.com` 的 **secondary validation** 连接 `47.116.10.200:80` 超时。该失败没有保存新配置。
- 再次执行配置演练成功，保存配置并实际重载 Nginx。
- Nginx 正常监听 80，两个域名均有独立 ACME 路由；本机 UFW 未启用，iptables INPUT 为 ACCEPT。成功到达的验证请求返回 200。证据支持公网连接偶发失败，尚不足以断定是安全组、云防火墙或线路中的哪一项。
- 已请求操作者提供该实例的安全组 80 端口入站规则截图及云防火墙／海外限制信息。当前没有阿里云控制台连接或云 API 凭证；未改变云端安全规则。

HTTP-01 要求公网验证节点能访问 80 端口，Let’s Encrypt 不提供固定验证 IP 白名单。继续使用本次 webroot 方式不需要阿里云 AccessKey。[Let’s Encrypt 集成指南](https://letsencrypt.org/docs/integration-guide/#firewall-configuration)

旧域名 `agent.xpersonatoy.com` 使用 `manual/dns-01` 且没有认证 hook，是先前两项定时任务失败的原因之一。它当前未到续期时间，本次未修改其认证方式；该旧域名的无人值守续期不属于上述两个域名的验收结果。

## 后续核对与回退

收到云端规则信息后，核对 TCP/80 的验证可达性及地域限制，再验证此前失败的多地点访问路径。不要通过跳过 TLS 校验或把测试证书装到正式站点来处理。

若需撤回本次配置变更：恢复备份的 `starrtoylumi-agent.conf`，并按备份的 timer 状态恢复旧定时器。回退会恢复原来的重复调度和缺少共享证书 reload hook 的状态，因此不应作为正常运维步骤。证书和私钥本次均未替换，无须回退它们。

本次仅运行短时 SSH/Certbot 验证命令，没有遗留临时监听服务。保留的 Snap 定时器是原有自动续期服务。
