# 正式站观测与临时监控并行

2026-11-13。适用于 47.116.10.200 的 `prod-20261112-2311`，正式配置 `/etc/powerx/config.yaml`、数据库 `powerx_pro`。本次是启动观测与完成升级后的定向修复，不代表自动清理、远端归档或旧执行器接管已启用。

## 两套监控各自的职责

临时容量任务使用 `capacity.py --apply --observe-only`：只采集、记录和向原私有飞书个人通道告警，不修改计划开关或删除数据库行。旧默认写入模式及数据库期限脚本仍保留 commit 保护。原文件日志守护和每日 00:05 磁盘日报不停止。

PowerX 在 Core 生命周期内每五分钟采样磁盘、当前数据库、系统表及已登记目录。使用正式安装、主机、文件系统和 PostgreSQL 集群身份，私有运行目录与受管存储同盘并由 Core 用户持有。`allow_enforce`、`admission_enabled`、`manage_native_retention`、`advanced_operations_enabled` 均保持 false；这是明确的观测模式。

临时通知 `api_accepted` 只证明飞书 API 接受；用户实际看到消息须另行确认。Core 健康 200、timer active、任务退出 0 或表存在，均不能单独证明自动清理已执行。

## 正式配置入口

`backend/scripts/ops/configure-production-storage.py` 默认只生成带原配置摘要的计划，拒绝 dev、非 PostgreSQL、非回环连接、错误的确认数据库，以及已有治理配置／运行目录。URL DSN 查询参数中的 `host`、`hostaddr`、`dbname`、`database` 覆盖一律拒绝，包括空值和编码后的参数名；计划和应用都会重新检查，旧工具生成的有效签封也不能绕过。普通连接选项如 `sslmode`、`application_name` 保留。开发入口保持原有 prod 拒绝行为，不能拿 dev 计划改路径后应用。

先核实真实主机、文件系统 UUID、`pg_control_system()` 的集群 ID 和实际 Core 服务用户，再从仓库根目录准备：

```sh
python3 backend/scripts/ops/configure-production-storage.py --config /etc/powerx/config.yaml --confirm-database powerx_pro --storage-root /opt/powerx/storage --host-id HOST_ID --filesystem-id FILESYSTEM_ID --postgres-cluster-id CLUSTER_ID --service-user ubuntu --output-plan /PRIVATE_DIRECTORY/storage-plan.json
sudo python3 backend/scripts/ops/configure-production-storage.py --apply-plan /PRIVATE_DIRECTORY/storage-plan.json --backup /PRIVATE_DIRECTORY/config.before.yaml
```

上面三个身份占位值必须来自目标机器，不复制 dev 的安装身份或执行账本。应用前重新核对原文摘要、属主、权限、设备和计划正文，保留私有原配置备份，再原子追加治理区块；不改其他配置的注释、凭据或序列化形式。配置变化、已有运行目录或不明旧状态要求重新审阅，不清空账本强行重跑。

脚本不重启日常服务。正式 Web 启动辅助脚本 `backend/scripts/ops/start-production-web-admin.sh` 以配置中的直接 backend port 和 web_admin_port 启动，默认 8080／3000；公开 HTTPS／WSS 地址与 SSR 回环分别设置。现场安装及 Core/Web 重启由维护者显式执行，Agent 核对运行结果。

## 本轮实际验收

- 用户已应用独立正式站安装包并重启 Core/Web；有效配置 SHA256 为 `e7a7c7b9496634409096a5a505f9b812d8585deb45f41eb1de3d6cd27f5db811`。
- Core 自动写入 23:55、00:00、00:05 时段的采样；每份包含磁盘、数据库、68 张系统表和 5 类目录，数据库 problem 为空。时段与实际采集时间分别记录，不把页面缓存时间当成任务执行时间。
- 临时监控重复执行退出 0，命令包含 `--observe-only`；原每日通知、文件日志和旧期限 timer 均保留。旧期限任务仍因版本保护拒绝执行，不把它写成正常清理。
- Web 使用正式 helper，SSR 指向 127.0.0.1:8080，公开地址为正式 HTTPS／WSS 域名。Core/Web 重启后没有再次自动重启；可选 Runner 缺包保持 inactive，on-failure 配置不形成重试循环。

能力目录、事件权限、工厂绑定和图片修复分别保存私有前后快照与回执。它们是独立升级任务，不能用监控采样成功替代插件安装、实际授权及图片文件验收。

### 11 月 13 日凌晨收口回执

六类服务器处理及核验完成，脱敏结果见[正式站验收记录](evidence/production-followup-validation-20261113.json)。用户已安装并启用 AI Craft 0.1.78；13 个主题、映射和插件 publish 授权完整，当前凭据具备 enqueue／consume。观察到实际 consume 请求成功，不能据此宣称业务消息已处理或支付链路已验收。

123 份原稿和 369 份派生文件的大小、校验值及图片约束通过，127 条插件图片记录已就绪；原素材引用保留。历史工厂重复绑定只删除 1 条，保留配置一致的有效绑定和其他业务记录。

00:50 前 Core 已连续写入 12 份五分钟采样，每份覆盖 68 张系统表和 5 类目录。23:55–00:50 数据库占用增加 16,769,024 字节；这是短时增长证据，不是长期容量保证。确认无编译任务后，使用 Go 原生命令清理约 7.75 GiB 可重建编译缓存，00:53 可用空间约 14.6 GiB；没有删除日志、数据库、备份或原稿。Core/Web 进程保持健康且没有新增自动重启；临时监控任务和 00:05 日报均退出 0。

用户删单后看到的 `asset_record_missing` 是插件清理账本中的独立历史异常。对应 UUID 在同租户插件及 Core 素材表均无记录，缺少可验证文件身份；保留异常与来源，不补造素材或冒充清理成功。插件的“已阅／收起”只是展示偏好，后续安装前端修复后验收，不能代替文件核对。

## 后续与回退

### 11 月 13 日 13:33 只读复核

正式站与开发站均已更新到 `20261113-1002`，Core／Web 服务 active，内网及公开健康接口返回 200，AI Craft 0.1.78 的最新安装任务和迁移均成功，没有 queued／running 安装任务。这里验证运行与安装状态，不代替登录后的订单、支付或删除验收。

根分区总量 117.74 GiB，已用 100.13 GiB，普通进程可用 12.61 GiB，另有 5 GiB 文件系统预留。正式库约 41.99 GiB、开发库约 5.24 GiB，主要占用来自调用审计和调用通知表；正式存储目录约 18.72 GiB，包含 8.49 GiB 快照、7.26 GiB 备份、2.75 GiB 归档。Go 编译缓存已重新增长到约 2.40 GiB。父目录、子目录和 SQL 数据库大小不能相加；部分 root 私有目录不能通过当前用户完整核算。

两站最新各 5,000 条调用样本中，超过 99% 来自 AI Craft 队列消费。企微、Shopify、LLM 请求的等待参数分别为 50ms、250ms、2000ms；每次调用产生 Core 审计及通知记录。开发库 01:35–13:30 分配空间增加约 1.54 GiB，正式库 01:35–13:25 增加约 179 MiB。审计副本省略消息正文，不能据此断言每次都是空取；本次未消费业务队列。

Core 两站采样仍覆盖 68 张表和 5 类目录；临时监控和文件守护 timer active、最近服务退出 0，日报下一次为 11 月 14 日 00:05。旧数据库期限服务仍 failed，新治理仍仅观测。当前 ubuntu 无权读取 root 私有通知回执及失败日志，本轮没有确认新的飞书送达或重新断言旧期限任务的具体失败原因。应优先按正式 TaskBus 阻塞合同降低插件消费频率，再预览并验收 Core 保留策略的受控执行；本轮没有删数据、重启或放开执行开关。脱敏数据见[只读复核记录](evidence/server-capacity-readonly-audit-20261113.json)。

现阶段新自动删除、远端归档及旧执行器交接均未启用。备份／快照若此前暂停，必须先核对原所有权回执和当前任务峰值余量；不因为监控恢复就直接开启。在 UI 发布安全保留策略并看到实际执行回执后，才讨论相应职责的交接。

保留旧私有恢复副本、水位、预留账本和正式备份，不删除目录或改旧 commit 白名单绕过保护。配置应用后若需要回退，应先核对已启动的进程和新账本，按维护流程处理；不盲目覆盖配置或删除新增的运行记录。

明早的具体操作顺序：先安装插件前端修复候选包并验收已阅、刷新和现有图片；再由 Root 打开“监控中心 → 存储治理”，核对正式安装身份、最新采样与未启用的期限策略。自动治理启用前，必须确认候选范围、审计保留要求、执行预算及现存恢复点，并先查看预览。

`allow_enforce=false` 和 `manage_native_retention=false` 仍是受信任部署门禁，Root 在 UI 保存策略也不能越过它们。完成部署级执行许可评审后，还需要部署操作者受控修改正式配置、按原流程重启日常服务，再由 Root 发布具体策略并核对首次真实执行回执；不能把当前 UI 保存成功写成清理已生效。本轮没有代替维护者开放这些开关。当前空间仍低于预留 20 GiB，不直接恢复备份／快照；取得足够峰值余量及正式执行回执后，才评审恢复和职责交接。无 OSS 目标时保持归档关闭，不向本机 Mac 静默推送。
