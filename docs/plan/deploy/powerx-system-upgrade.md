# PowerX 已安装实例升级计划

日期：2026-10-11。状态：**Core 合同、配置护栏、迁移 CLI/履历/锁与发布门禁已实现，本地原生旧源码升级通过；真实镜像 CI 与可执行发布制品尚待分支提交后生成。**

正式实现：[升级合同](../../contracts/powerx-installed-upgrade.md)、[实施证据与边界](../../contracts/repairs/installed-upgrade-20261011.md)。设计草案以正式合同 v1、JSON Schema 和实现为准。

目标：已完成 Setup 的旧实例升级到新镜像后，沿用原配置、数据库、账号、密钥、上传文件与插件，通过增量迁移获得新功能，全程不重新提交 Setup。

关联：[部署总览](./README.md)、[025 spec](../../../specs/025-powerx-docker-systemd/spec.md)、[025 plan](../../../specs/025-powerx-docker-systemd/plan.md)、[025 tasks](../../../specs/025-powerx-docker-systemd/tasks.md)。

## 1. 范围与协作边界

| 负责方 | 交付内容 |
|---|---|
| PowerX Core | 发布身份与成对镜像元数据、兼容范围、配置升级规则、迁移计划/执行/状态 CLI、增量系统数据、旧实例升级 CI、回滚条件及证据 |
| XDocker | 根据正式合同实现备份、维护状态、迁移调用、镜像切换、升级页面及任务记录 |
| 插件 / Framework | 配合已声明的兼容版本迁移与实际插件运行验收 |

实施使用独立任务分支，保留当前工作区已有改动；发布需经提交、CI 与正式验收，不直接推送共享 develop。PowerX 的交付通过发布制品和 CLI 完成，本需求不新增 Core 升级管理网页，也不修改 XDocker 或操作当前服务器。

应用版本升级、实例从 A 搬到 B、插件版本升级分别按本计划、[实例迁移指南](./powerx-instance-migration.md)、[插件升级 SOP](./plugin-upgrade-sop.md)处理。

## 2. 现有基础与差距

以下为 2026-10-11 源码核对结果，不是目标功能的验收声明。

| 现有实现 | 可复用部分 | 差距 |
|---|---|---|
| [init.py](../../../deploy/powerx/container/init.py) | 已有 config.yaml 时保留原文件，不重新生成密码与密钥；部分初始化目录会报错 | 仅检查存在；缺少明确升级模式及配置可读、合法、安装状态和挂载校验 |
| [entrypoint.sh](../../../deploy/powerx/container/entrypoint.sh)、[run.sh](../../../deploy/powerx/container/run.sh) | 已有 migrate/status 入口；serve 要求配置非空；没有启动时自动重装 | start 仍调用 init；升级需明确禁止在缺失旧配置时生成新实例 |
| [database CLI](../../../backend/cmd/database/main.go)、[MigrateDatabase](../../../backend/cmd/database/migrate.go) | 结构迁移、Agent 迁移、部分数据回填及扩展表；失败退出非零 | 迁移与 seed 分离；没有统一目标版本计划和逐项执行结果 |
| [IAM UUID 迁移](../../../backend/pkg/corex/db/database/iam_phase2_uuid_migration.go) | database_migration_records、稳定 migration_id、事务内回填与完成记录 | 仅覆盖部分迁移；需扩展统一履历、校验和、失败尝试和全局执行锁 |
| [database status](../../../backend/cmd/database/main.go) | JSON 表状态及有限检查 | 不等同于目标版本的迁移状态、阻塞清单或执行报告 |
| [镜像 CI](../../../.github/workflows/publish-container.yml) | 同 SHA 构建 backend/web-admin，声明 amd64/arm64，空库安装与登录烟测 | 未发布完整升级执行契约；缺少固定旧镜像到目标镜像的跨版本 CI |

部分旧数据存在明确迁移阻塞。例如 [plugin release 服务身份迁移](../../../backend/pkg/corex/db/migration/202609060001_plugin_release_service_actor.go)遇到缺少 plugin_id/service_actor 的历史记录会失败，且失败前可能已加列。必须在计划阶段检查，并提供保留历史数据的处理与恢复办法；不能通过自动删除记录或伪造服务身份放行。

## 3. 必须保持的实例状态

1. `install.status=installed` 保持不变；升级后进入登录或正常应用页面，Setup 写接口继续被安装保护拒绝。
2. 保留管理员/用户 UUID、密码 hash、租户、业务数据、用户修改的配置与权限。
3. 保留 PostgreSQL/Redis 密码、JWT、加密、签名和存储 token 密钥，以及插件凭据、Registry、安装文件及数据库绑定。
4. 保留域名、端口、存储驱动、路径、桶、文件内容和下载/检索所需关联信息。
5. 发布通道与 `deployment.env` 分离；新镜像通道不得更改实例环境、插件 Role/User 命名或绑定。
6. 账号与密钥保留不代表旧浏览器 Token 永久有效。版本可明确要求正常续期/重新登录；不能要求重新创建管理员或再次 Setup。
7. 插件状态保留须验证实际加载、关键 API、文件读取与数据库最小权限；仅验证 Registry 行数不足以通过。

禁止通过 refresh、完整 Setup、完整初始化 seed、删除配置/数据目录、重建空卷或轮换密钥实现升级。

## 4. 发布升级元数据（待评审）

拟交付 `powerx-upgrade-manifest.json` 与 JSON Schema。XDocker 候选目录继续用于发现与预览；执行前必须读取并校验正式升级制品。

| 字段组 | 必须表达的含义 |
|---|---|
| schema_version / release_id | 合同版本和唯一发布身份；未知 schema 明确拒绝 |
| channel / version / source_sha | 开发/正式通道、版本、完整 40 字符源码 SHA |
| images | backend 与 web-admin 的不可变 digest、OCI revision、支持平台；多架构 index 与选定平台 digest 区分清楚 |
| supported_from | 已验收的来源版本/完整 SHA 及来源成对镜像 digest；不凭短 SHA、同名 tag 或模糊范围猜测兼容 |
| database | 迁移集合版本、migration ID/校验和/顺序/依赖，是否涉及结构、回填或系统数据，以及数据库/扩展前提 |
| configuration | 配置 schema 变化、新增默认、弃用项、必填人工输入、写入步骤及回滚需求 |
| compatibility | Core/前端协议、Framework/插件要求、会话更新要求及待管理员完成的动作 |
| execution | 是否必须停写/停进程、锁超时、非事务步骤和恢复策略 |
| rollback | 明确 image_only、restore_required 或 unsupported；允许回退的固定镜像对、数据库与配置条件 |
| verification | CI run/job、测试基线、目标 digest、实际测试平台、证据制品引用及校验信息 |

支持构建的平台与已通过升级测试的平台分别记录。仅构建成功不能宣称该平台升级已验收。失败或未验收的镜像可以保留排查，但不能进入可执行升级目录。元数据不得包含密码、JWT、私钥、DSN 或可用凭据。

同一源码 SHA 的镜像重建也以本次实际 digest 为准。数据库迁移版本与应用版本分开标识，避免把镜像 tag 当成迁移履历。

## 5. 配置升级规则

- 首次 init 与 upgrade 明确区分。升级要求现有配置及指定数据挂载可读、完整且有效；缺失、空文件、损坏 YAML、权限不足或挂载异常均阻断。
- 原 YAML、环境变量和受保护 secret 文件的有效配置优先级需要核对并写入合同；不能因换镜像、默认环境或新模板改变有效密钥。
- 新可选字段只按发布声明补默认值；新必填字段缺失时输出阻塞及需要输入的字段，不能随机生成密钥、猜测环境或默认指向新数据库。
- 需持久写配置时保存原文件，使用受保护的原子替换，保留 owner/mode；记录改动字段名，不输出字段秘密值。升级失败时保留 installed 状态及可恢复的配置版本。
- 数据库与文件配置不是同一个事务。合同必须明确配置准备、数据库迁移及应用切换顺序，以及各阶段失败的补偿/恢复办法。
- 本计划不升级 PostgreSQL/Redis 大版本；基础设施变化另行声明兼容条件与专门流程。

## 6. 统一增量迁移机制

### 6.1 注册与履历

复用既有集中迁移挂载和 database_migration_records 基础，建立统一注册表。每个迁移提供稳定 ID、内容校验和、依赖、适用来源、预检查、执行、后检查、事务策略和恢复说明。已发布迁移不可原地修改；履历校验和不匹配必须阻断，修正通过新 migration ID 交付。

旧库缺少完整履历时，先核对声明的来源及真实 schema/数据条件，再建立经验证的基线记录。不能仅因传入一个版本号就把所有历史迁移标记为完成。

执行尝试使用 UUID 标识，关联发布身份、计划摘要、数据库/schema、开始/结束时间及结果。技术 migration ID 保持稳定字符串；对外执行记录不新增 numeric ID 引用。

### 6.2 并发、事务与失败

- PostgreSQL 执行锁绑定实际目标库/schema，全程持有；连接断开释放，支持有限等待与稳定退出码。每次 apply 取得锁后重新检查计划及履历。
- 迁移锁只协调迁移进程。需要停写的发布必须由 XDocker 停止 Core、Worker 及其他会写入的进程，禁止新旧版本混跑造成 schema/授权竞争。
- 可事务步骤将变更、回填、校验和成功记录一同提交；失败回滚且不记成功。失败尝试记录单独保留。
- 非事务步骤明确 checkpoint、已发生影响、可安全重试范围及恢复要求，例如并发索引、外部库操作、文件转换。不得把多个存储操作描述为全局原子事务。
- 全部成功返回 0；阻塞、锁冲突、执行失败、部分完成或需要恢复返回非零。进程被终止后不得遗留“成功”记录，重试先校验实际状态。
- 初期只支持已验证的离线升级。在线迁移必须有单独声明和并发写入测试。

### 6.3 结构与系统数据

既有 AutoMigrate 可作为经评审的结构步骤复用；每次运行全量 AutoMigrate 不等于具备版本迁移和回滚保证。

新增菜单、权限、能力定义、必要内置数据使用版本化的专用步骤，按稳定对象标识及平台拥有字段补齐。保留用户修改、管理员密码、现有 Key grants、插件 owner 和租户能力版本锁。发布目录同步不代表自动授权；需管理员确认的能力变化列为 post-upgrade action。首次安装的完整 seed 保留其原用途。

## 7. 迁移 CLI 与 JSON

现有命令是 `/app/database migrate` 与 `/app/database status`。建议在这个二进制上增加三类操作，最终命令名、参数和 JSON Schema 在实施阶段由 Core 与 XDocker 确认：

| 操作 | 行为 |
|---|---|
| status | 只读查看数据库迁移履历、未完成尝试与当前基线；不创建表或补数据 |
| plan | 只读评估目标发布；输出已执行、待执行、阻塞、配置输入、维护和回滚要求 |
| apply | 显式执行指定目标及确定计划，取得锁后重新校验，输出逐项成功/跳过/失败及恢复动作 |

以下参数已实现；plan/apply 还必须提供正式合同规定的 `--source-sha`、现有配置/挂载路径，apply 提供 `--confirm-offline`：

```text
/app/database migrate --action=status --format=json
/app/database migrate --action=plan --target-release=<release_id> --source-sha=<source_sha> --format=json
/app/database migrate --action=apply --target-release=<release_id> --source-sha=<source_sha> --expected-plan=<plan_hash> --confirm-offline --format=json
```

目标镜像自带迁移集合与发布身份，不能从调用方接受任意 SQL、任意脚本路径或不受信任的迁移 URL。计划摘要用于发现预览后的漂移；锁内重新验证结果才可执行。

JSON 至少提供：schema_version、operation、run_uuid、source/target release、数据库迁移版本、plan_hash、status、applied/skipped/pending/failed、blockers、配置变更、是否部分完成、retryable、requires_restore、错误码、脱敏说明与关联 ID。具体字段类型及枚举在正式 Schema 固定。

标准输出只输出可解析 JSON；日志写标准错误，保留确定退出码。结果及日志均不得输出凭据、完整秘密配置或未脱敏 DSN/SQL 参数。当前 status 的表统计不能作为上述计划/执行结果替代。

## 8. 旧实例真实升级 CI

首轮来源源码基线：`3d792fd7ea2426825970bceceb0d6e9db46211ef`。先核对并固定该发布的 backend/web-admin 镜像 digest，当前计划没有宣称其实际镜像已完成核验。找不到历史镜像时，历史源码重建测试单独标记，不能当作真实已部署制品升级验收。

| 阶段 | 必须验证 |
|---|---|
| 旧实例准备 | 使用固定旧镜像完成 Setup；自定义管理员密码，建立租户/样例业务数据、上传文件并安装启用样例插件 |
| 保存基线 | 私有配置、密钥、密码 hash、数据关系、文件/对象存储内容、插件版本/状态/数据库绑定；校验信息脱敏 |
| 预检查 | 成对 digest、来源、平台、扩展、配置、挂载、阻塞及计划漂移；未满足条件不执行 |
| 迁移与启动 | 停写/停止旧写入进程，目标镜像显式迁移，启动目标镜像；不调用任何 Setup 写入口 |
| 连续使用 | installed、旧账号密码登录、数据及文件读取、密钥和有效配置保留、插件实际运行及新功能可用 |
| 幂等与并发 | 第二次 apply 无重复回填/种子副作用；两个迁移进程争锁行为明确 |
| 故障注入 | 在部分迁移已完成后失败及进程终止；失败不记成功，JSON/退出码正确，按声明重试或恢复 |
| 拒绝场景 | 缺失/损坏/不可读配置、缺数据挂载、未知来源、修改旧 migration 校验和及历史阻塞数据均明确失败 |
| 回滚 | image_only 必须真实运行旧镜像并测试读写、登录及插件；restore_required 必须恢复匹配数据库/配置/必要文件后再运行旧镜像 |

测试只使用隔离实例和测试凭据，不读取或操作当前服务器。CI 保留失败日志、逐项 JSON 和测试平台；成功门禁只认证实际测试的镜像对、来源与平台。现有空库安装测试继续运行。文件备份涉及对象存储时同时验证对应驱动，不能仅验证本地目录。

## 9. 回滚声明

每个发布明确规定：

- `image_only`：升级后的数据库、配置和文件格式兼容指定旧镜像，且已完成真实回退读写验证。
- `restore_required`：必须先停止新版本写入并恢复升级前的数据库、配置及声明的文件状态，再回退指定镜像对；说明升级后新增数据的保留/丢失窗口。
- `unsupported`：没有安全回滚路径时阻断普通升级流程，提供专门处置计划。

未验证回滚的版本不能标成“可直接回滚”。恢复期间所需权限、旧镜像、扩展与资源要求均写入发布制品；XDocker 负责实际备份和切换任务，Core 提供判断依据与验收。

## 10. 实施顺序与交付清单

| 阶段 | Core 交付 | 完成条件 |
|---|---|---|
| P1 合同与审计 | 正式升级合同、元数据/CLI JSON Schema、现有迁移清单与来源范围 | Core/XDocker 确认执行合同；每个字段有明确来源和状态 |
| P2 配置护栏 | 严格 upgrade 模式、持久配置规则与兼容补丁 | 旧配置及密钥保留；缺失/损坏/挂载异常阻断 |
| P3 迁移机制 | 通用履历、校验和、锁、只读 status/plan、显式 apply | 重复执行、并发、失败和中断行为通过测试 |
| P4 增量补齐 | 目标版本结构/回填/系统数据步骤及插件兼容方案 | 新功能可用；用户设置/密码/授权和历史数据保留 |
| P5 升级验收 | 固定旧镜像到目标镜像的 CI、幂等/故障/回滚证据 | 所有强制场景通过，支持范围与平台准确 |
| P6 发布交接 | 已验收成对 digest、机器元数据、固定命令、报告及回滚说明 | XDocker 可依正式合同实现编排与任务记录 |

本计划的任务跟踪见 025 的 Phase 14（T140–T157）。正式实施前创建独立任务分支。发布命令、制品或 CI 尚未实现/执行的项目保持未完成，不能用本文中的设计示例宣称已经支持。
