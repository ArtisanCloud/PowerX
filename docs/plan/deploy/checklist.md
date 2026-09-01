# 上线验收与演练清单

## 1. 上线前检查（一次性）

- [ ] `config.yaml` 已启用插件并落到持久目录
- [ ] `config.yaml` 已显式配置 `deployment.env=prod`，且未从 `version`、`dev_mode`、目录名或安装元数据推导
- [ ] `CORE_X_AUTH_JWT_SECRET` 已替换为生产密钥（长度>=32）
- [ ] PostgreSQL / Redis / MinIO 连通性已验证
- [ ] Nginx 同域反代配置已加载并生效
- [ ] backend 与 web-admin 健康检查接口可访问

## 2. 首次发布验收（应用层）

- [ ] 首页、登录、管理后台主要页面可访问
- [ ] `/api/v1/health` 返回 200
- [ ] `/_p/<plugin_id>/admin/` 可访问
- [ ] 宿主与插件会话桥接正常（Authorization 透传）

## 3. 插件升级验收（核心）

- [ ] 安装前确认 `deployment.env` 与已有插件 Registry 记录一致
- [ ] 完成一次 `install/local(enable=false)`
- [ ] 完成一次 `switch_version(enable=true)`
- [ ] 完成一次 `switch_version` 回滚到 N-1
- [ ] 升级与回滚动作均有审计与日志记录
- [ ] 插件 Schema/Database 名称保持现有规则，Role/User 带 `prod` 环境段，账号无法访问非目标数据库、Schema 或其他插件对象

## 4. 观测与告警

- [ ] 指标可见：`plugin_release.*`
- [ ] 指标可见：`powerx_capability_invoke_total`
- [ ] 关键告警已配置并可触发演练
- [ ] 升级窗口内日志与 trace_id 可追踪

## 5. 灾备与回滚演练

- [ ] 应用版本回滚（Docker tag 或 systemd 软链）演练通过
- [ ] 插件版本回滚（switch_version）演练通过
- [ ] 数据库备份恢复演练至少完成一次
- [ ] 关键目录（plugins/registry/installed）已纳入备份策略

## 6. 交接资料

- [ ] 本目录文档已纳入运维知识库
- [ ] 值班同学掌握标准发布与回滚命令
- [ ] 故障升级路径（研发/运维/平台）已明确
