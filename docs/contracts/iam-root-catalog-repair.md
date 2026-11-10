# 系统 Root 权限目录 UUID 冲突：定向修复

日期：2026-11-09。适用于历史 `iam_role_permission` 中数字主键与 UUID 关系不一致，导致系统 Root 当前请求可依赖特权访问，但历史授权基线返回 `IAM_BASELINE_ROOT_BYPASS_UNSUPPORTED` 的现场。本次不改 Framework 合同、插件历史窗口、普通成员角色或 API-key／STS grants。

Core 审查修正的源码、隔离／PostgreSQL 回归、只读现场预览及未执行边界见[本轮验证收据](evidence/iam-root-catalog-repair-core-fix-20261109.json)。旧验证记录不代替本轮 v2 计划。

## 本机执行结果（2026-11-09）

用户确认后，已对本机开发库 `powerx` 执行一次独立维护工具。执行前重新核对配置只指向本机、生成 v2 预览，并备份六张相关授权表。计划仍为 523 条候选、0 条阻断、2 条历史升级；执行与已审阅指纹一致。

执行后只读核验：Root 错配由 523 条降为 0；小样、大货两项授权均为 `allowed`、`baseline_kind=native`，事实版本由 11 升为 12；原 `known_since` 保持。非 Root 角色权限，以及非目标授权流和历史段的执行前后摘要完全一致。再次预览的候选、阻断及待升级历史均为 0。执行详情见[本机落库收据](evidence/iam-root-catalog-repair-local-apply-20261109.json)，数据库备份及完整计划仅保存在受限目录。

此次只修复本机数据，没有运行 seed、重启 Core／插件、部署新构建或修改远程服务器。运行中 Core 的产品接口仍返回 200。浏览器连接不可用，真实 Root／普通成员的页面结果尚待刷新验收；数据库回查不能代替页面验收。恢复段从实际执行时间开始，不倒填旧授权时间或重新生成恢复前通知。

远程服务器升级须先使用自己的配置独立预览：存在同类候选时，经备份并核对该实例的计划后显式执行；没有候选则不需修复数据。不能复制本机指纹或假设远程也有 523 条。`make migrate`、seed 和服务启动均不会自动执行此维护工具。

## 根因与源码修复

### 已复现的身份变更入口

`SeedBuiltInRolesAndGrants` 原先对已有 `root`、`system_monitor` 使用通用 `BaseRepository.Upsert`。创建候选对象时 `PowerUUIDModel.BeforeCreate` 会生成新 UUID，而通用冲突更新字段包含 `uuid`，因此第二次初始化会保留数字 ID、覆盖 UUID。已有 `RolePermission` 不会随之重绑，重复赋权又会撞上数字复合主键而被忽略。

隔离测试已复现：同一个角色第二次 upsert 后数字 ID 相同、UUID 不同；按数字 ID 读取能得到权限，按新 UUID 读取得到 0 条，旧 UUID 已没有角色归属。这能制造当前同类错配；它不能单独证明日常库的 523 条具体由哪次执行产生，也不能证明某个旧安装包当时走了哪条读取路径。

初始化入口现改用 `RoleRepository.UpsertPreservingIdentity`：冲突仅更新明确的元数据字段，保留 ID、UUID、创建时间，并回读数据库中的真实角色，覆盖调用方对象里未落库的临时 UUID。Root 和系统监控角色均使用此路径，租户默认角色及自定义角色不被替换。回归通过完整角色初始化重复执行、五种角色同时拥有读取权限、已有悬空关系不被自动更正，以及 SQLite／独立 PostgreSQL 验证。

本项修正防止继续制造错配，不等同于修复存量数据或恢复页面。角色身份稳定性收据记录的是源码验证当时尚未执行数据修复的边界；本机后续执行状态见上节。还须核验 Root 的既有查看范围、旧变更记录与通知时间下界的关系，不能仅以 503 消失作为业务恢复完成。验证见[角色身份稳定性收据](evidence/iam-role-seed-identity-fix-20261109.json)。

目录关联以 `(role_id, permission_id)` 为主键，另有 UUID 唯一键。若数字角色仍是当前系统 Root，`role_uuid` 却指向不存在的旧角色，`rootrole.Ensure` 的正确新增会撞上数字主键。原 `ON CONFLICT DO NOTHING` 把冲突当作成功，留下 UUID 读取路径无法消费的关联。

显式目录同步仍先检查 Root 所属关联的数字／UUID 一致性，写后验证当前有效权限均已关联；冲突明确返回 `IAM_ROOT_ROLE_CATALOG_INCONSISTENT`。不在安装、启动或迁移中猜测重绑旧 UUID。

正常启动和普通授权源提交仅对这一明确的一致性错误作隔离：普通成员的初始化、停用及角色撤权继续完成；仍具有 Root 身份的历史保持原样，不借无关事务重算。Root 身份被撤销、账号／成员失效时仍正常记录拒绝。其他数据库或账本错误继续回滚，不能统统吞掉。启动通过只读 `rootrole.CheckCatalog` 输出维护警告；受影响的 Root 查询继续按正式账本返回不可用，不以 `is_root` 绕过。健康目录的幂等同步与历史算法不变。

## 独立修复的范围

工具为 `backend/cmd/root-catalog-repair`，服务入口 `authorizationbaseline.RepairRootCatalog`，当前合同为 `core.iam.root_catalog_repair.v2`。旧 v1 预览的指纹不能用于 v2 执行，必须重新预览。无 HTTP 自动修复入口，不被 migrate、启动、seed 或查询调用。

仅当以下条件同时成立，关联才进入候选：当前角色是系统内置 Root；数字角色 ID 匹配它；旧 `role_uuid` 非空且在全部角色中均无归属（含已删除角色）；权限数字 ID 对应的权限 UUID 与该关联保存值相同。更新只改变这条目录关联的 `role_uuid`，保留权限身份、状态、元数据、原创建时间及非 Root 行。

若 UUID 仍属于有效／已删除角色、权限 UUID 不一致、数字角色 ID 冲突或目录变化，整个执行拒绝，不用数字 ID 覆盖另一份有效结构化身份。

预览在只读一致性快照中生成目录与授权历史计划。`history` 逐条列出会变更的租户、插件、Root 成员、权限、stream UUID、前后事实版本、前后事实摘要及保留的 `known_since`。`history_fingerprint` 覆盖原 tracking、stream 和全部原历史段；无法匹配已知旧来源的记录列入 `history_blocked`。

预览仅在内存投影候选关联，不写目录或账本。完整指纹包含合同版本、目录和上述历史计划；新增受影响 Root、历史版本／历史段／追踪起点变化都会使旧计划失效。执行必须提供维护人及已审阅指纹，并在 Core 共用授权源锁内重新读取、匹配、逐行 CAS，只升级计划列出的 stream，再核对目标事实版本和已保留起点。指纹不匹配需重新预览并重新核对，不自动接受新计划。

更正关联后复用现有精确旧指纹 Root 来源升级，从事务实际时间追加新授权段，保留原 `known_since`、旧历史和已有连续区间；不能把恢复时间倒填成注册、登录或旧订单事件时间。普通成员不做漂移重算，插件事件、游标、投递与业务数据不访问。

## 执行顺序

先让 Core 维护方 review 本轮源码及隔离测试，再用新的维护工具预览。以下命令从 Core 仓库根执行，默认配置相对于 `backend/`。

1. `make root-catalog-repair ROOT_CATALOG_REPAIR_CONFIG=etc/config.yaml`：只读预览，检查 `contract` 为 v2，并审阅 `root_uuid`、`total_links`、`candidates`、`blocked`、`history`、`history_blocked`、`fingerprint`；`applied` 应为 false。保留输出为受限访问的预览记录。配置加载日志可能出现在计划之前，不将整份日志冒充纯 JSON 文件。
2. 只有目录与历史范围均已审阅、`blocked` 和 `history_blocked` 均为空且有执行授权时才执行：`make root-catalog-repair ROOT_CATALOG_REPAIR_CONFIG=etc/config.yaml ROOT_CATALOG_REPAIR_ARGS='--apply --expect-fingerprint FINGERPRINT_FROM_PREVIEW --operator MAINTENANCE_OPERATOR'`。替换两个大写占位值，不能使用旧收据的指纹。
3. 保存执行收据，检查 `applied=true`、`repaired` 与已审阅关联数一致；核对 `history` 的 stream 事实版本和保留起点。若返回计划／归属／历史冲突，停止并重做只读诊断；不要手动删角色、重置基线或盲目重试。
4. 数据修复提交后即生效，不因执行本工具而要求重启。若还需部署本轮 Core 源码，维护方应另行重新编译，日常服务由用户启动；不能把重启当作数据修复步骤。本次插件和 Framework 不需重新打包，也不需要运行插件 migrate。
5. 用真实 Root 和普通成员补验：Root 小样／大货基线应返回正常决定及真实 `visible_since`；打开订单内容变化不再 503。普通成员、历史窗口、成本裁剪及已读状态保持。用专用测试单在恢复后新增一项变更，确认 Root 能按正常流程读取并确认；不倒填旧授权以强行展示恢复前历史。

部署本轮 Core 后，坏 Root 目录不会再阻止整个启动或普通用户停用／撤权，但重启也不会自动修好这批旧关联。受影响的 Root 基线恢复仍需单独审阅、执行维护计划。修复工具仅连接数据库并执行指定维护入口，不启动 Core、插件或通知 worker。

## 回滚与恢复

目录更新和 Root 历史追加共用事务；任一 CAS、归属核对、账本更新或后置检查失败，全部回滚。提交结果不明时先重新预览并核对历史，不自动重放写入。成功后不通过把 UUID 改回无效值或删除历史段来“撤销”；需要灾难恢复时由 Core 维护方使用执行前受保护的数据库备份，在正式维护窗口内按完整恢复流程处理。

预览及执行收据应受限保存，记录维护人、前后指纹、候选旧值、执行时间和结果。不得将配置、数据库密码或服务凭据放入日志和命令参数。

## 验证范围

受控覆盖：旧逻辑数字键冲突、严格目录拒绝、坏目录下启动／停用／撤权、Root 历史保持与 Root 身份撤销、有效／已删除角色归属拒绝、权限 UUID 冲突、目录／新 Root／历史版本／历史段／追踪起点变化、嵌套源写拒绝、未知历史拒绝、账本失败整笔回滚、Root 新授权段及普通成员不变。PostgreSQL 专项验证实际主键／UUID 约束、并发源事务和 523 条批量修复。本机日常库的后续落库另见执行收据，受控测试、实际落库和真实浏览器结果分别记录。
