# 已安装实例升级 Core 实施记录

日期：2026-10-11。首次实施分支：`codex/026-installed-instance-upgrade`；后续合入当前 `024-ai-engineering-skills` 分支。本次发布修复继续在当前分支进行。

## 已实现

- [升级合同](../powerx-installed-upgrade.md)、manifest/result JSON Schema、固定命令与退出码。
- 严格升级配置检查；init/serve/Go loader 不搜索替代配置、不生成密钥、不读取开发 .env、不重新 Setup。本版本不写配置补丁。
- 集中注册、内容寻址校验和、旧 IAM 履历兼容、UUID 尝试记录、只读 status/plan、锁内复核及 apply。
- 专用 PostgreSQL 连接执行锁；旧 migrate 与 apply 共用锁。事务变更和成功记录同提交，失败独立保留；非事务 Knowledge 失败要求恢复。
- 正式系统目录增量同步，保留权限/能力 UUID、effect/status、删除状态、用户描述/图、授权关系和已有租户版本锁。修复候选墓碑占用唯一键与 `metadata:tag:manage` 来源晋升，不复活候选、不执行整套 seed。
- 已安装实例正常启动时保留 Capability 身份与状态，避免启动目录同步覆盖保留结果。
- 固定真实旧镜像对；校验 OCI index/平台 digest/revision，新镜像 database 编译并回读发布 SHA。
- CI 保留空库安装，增加两架构的真实旧 Setup → 新迁移 → 凭据/业务/上传/插件回读 → 幂等/并发/失败/TERM/漂移/checksum/历史阻塞 → 备份恢复回滚。全部通过才生成 manifest 与发布包。

## 本地已执行验证

| 验证 | 结果与边界 |
|---|---|
| Python 初次 init | 2 项通过 |
| Python upgrade config/release | 6 项通过，含多组非法输入与制品拒绝；不是镜像验收 |
| Go database/config/数据库与权限相关包 | 通过；PostgreSQL 专项单独执行 |
| PostgreSQL runner（含 race） | 5 组通过；本机 PostgreSQL 14、专用 `powerx_upgrade_test_*` 库 |
| PostgreSQL Capability 保留 | UUID/ID、disabled、用户图及来源冲突验证通过 |
| 固定旧源码原生库 → 工作区 CLI | 4 步成功，第二次跳过全部；旧 migrate/seed 仅用于隔离旧库 fixture，不是 Docker 或 HTTP Setup 验收 |
| 配置/凭据/Profile grants/业务 | 摘要保持一致；目标使用 test-native / `aaaa…` 测试身份，不是发布 SHA |
| 真实 CLI JSON Schema | plan/apply/status 通过；[证据 JSON](../evidence/installed-upgrade-native-20261011.json) |
| Go vet、YAML/shell/Python 语法、diff | 通过；不代表 hosted runner 已运行 |

integration_gateway 的两个旧 media 测试要求已不存在的 builtin `com.corex.media.assets.manage`，整包测试失败；该定义未由本次升级修改。新增定向 PostgreSQL 测试通过，不以整个包全通过表述。

## 待生成的发布证据

首次实施时未执行真实镜像 CI。2026-10-11 后续核实：代码已由 PR #410 合入 develop，提交 `6a03034c686b66ca4b39866423a23a84edeaaa3d` 的 [发布 CI #38131287774](https://github.com/ArtisanCloud/PowerX/actions/runs/38131287774) 已执行，结果为失败。

- `images` 通过：迁移单测、双架构镜像构建、成对 OCI digest 核对及空库安装烟测成功；`powerx-target-pair` 产物已生成。
- `installed-upgrade` 的 amd64、arm64 均失败：旧镜像 Setup 的配置、Provision、Complete 请求为 200；报告停留在重启后的安装状态等待，未通过 `old_setup`，迁移结果为空。
- 后续 `upgrade-metadata`、`bundle-release` 被门禁跳过。当前候选 digest 是排查产物，不是通过升级验收的发布声明；可执行 manifest 和本次发布包尚未生成。
- 两个平台的失败报告仅给出 `UPGRADE_CI_ACCEPTANCE_FAILED`，不足以证明具体异常。已补齐阶段、异常类型和代码位置，并修复就绪检查把 Nginx HTML 502 错误页直接当 JSON 解析的问题；仅对 502/503/504 和连接异常有限等待，403 和无效成功回包直接拒绝。相关 Python 测试 11 项通过，仍需新一轮真实 CI 确认。

上述远端运行与产物记录见 [CI 核实证据](../evidence/installed-upgrade-ci-20261011.json)。本次核实通过 GitHub 连接器读取日志和报告，没有启动本机 Docker，也没有操作业务服务器。后续交付须记录通过验收的新 SHA、成对 digest、manifest 校验和、两平台报告和发布包。

后续修复运行 [#38138761396](https://github.com/ArtisanCloud/PowerX/actions/runs/38138761396)，目标 SHA `39d1c26403dd5e578c9b0f81b7d6f91047d93020`：镜像与空库验收通过；报告明确记录了重启期间的 502，随后旧安装、登录和 Scheduler 测试通过，确认就绪等待修复有效。升级测试在宿主创建测试插件目录时发生 `PermissionError`：容器生成的私有目录属于 root，GitHub runner 无权写入。已增加只对 `powerx-upgrade-*` 隔离 CI 的两处挂载进行所有权交接，保留文件 mode、不跟随符号链接；生产初始化和升级路径未放宽权限。Python 验收测试现为 13 项通过。迁移尚未执行，发布门禁继续阻止生成 manifest 和发布包，等待下次真实运行。

第三轮 [#38139994601](https://github.com/ArtisanCloud/PowerX/actions/runs/38139994601)，目标 SHA `5b0af7a4a9053b5ca327c687a338295ae0839a1e`：镜像、空库和旧安装验收通过；目录修复生效，测试插件安装返回 200。随后 `/probe` 返回 403，迁移仍未执行。核对固定旧源码发现 fixture 使用了旧式 resource/actions 权限声明，登记为 action，缺少网关要求的 API 协议绑定；Root 也必须先命中正式路由登记。已把测试插件改为显式 `type=api`、独立 `upgrade.probe:read`、`GET /api/v1/probe` 的 Admin JWT 绑定，并在调用前验证真实权限登记。没有开放 public 路由或放宽 Core 授权。失败回包仅登记白名单错误码和原因，保留 request_id，避免记录凭据。

第四轮 [#38141125885](https://github.com/ArtisanCloud/PowerX/actions/runs/38141125885)，目标 SHA `257b0b8a42ed534dd2de310b9aaac02c9ffc980a`：API 权限登记检查通过，但探针被拒绝，响应明确为 `no registered permission binding for this route`。核对网关的 `normalizeGatePathForPolicy`：`/_p/{id}/api/v1/probe` 在授权时剥离 `/api/v1`，使用 `/probe` 相对坐标。fixture 的 protocol binding 已同步为 `GET /probe`；外部访问地址仍为原地址。已有网关路径归一化及上游路径构造测试通过，Core 网关未改动。Python 14 项测试通过；正式升级仍等待新一轮验收。

第五轮 [#38141655820](https://github.com/ArtisanCloud/PowerX/actions/runs/38141655820)，目标 SHA `8c9ff3c98225c4e4584b7190f1d670c001f1c5f5`：amd64 实际报告已有 18/19 项通过，包含迁移成功、数据/凭据/文件/插件保留、幂等、故障重试、中断恢复、锁冲突、非法配置/来源/履历拒绝及成对镜像核对。媒体 presign 回包的可选 `headers=null` 导致 Python 请求构造器 `AttributeError`，尚未进入最终下载及备份恢复回滚。已把可选空头部规范为 `{}`；不改签名 URL、凭据或服务端合同。仍保留 `restore_rollback` 必须通过的发布门禁。

第六轮 [#38142509247](https://github.com/ArtisanCloud/PowerX/actions/runs/38142509247)，目标 SHA `5876f641d5132c58b89048a3ed4746dd5c0f47f2`：构建完成，空库首次请求 `/setup` 时发生 `ConnectionResetError`，升级矩阵未执行。已增加只对连接异常和 502/503/504 的有界前端启动等待；403 不重试，后续空库、登录和升级检查全部保留。Python 16 项测试通过，待重新完成真实发布门禁。

第七轮 [#38142990684](https://github.com/ArtisanCloud/PowerX/actions/runs/38142990684)，目标 SHA `a2be4a3a0dc32180bf0058d2dc4b4116647f787e`：amd64 报告仍为 18/19 通过，实际升级/保留/故障恢复检查均通过；媒体下载沿用了 Admin legacy presign 返回的 `/media/{uuid}/resource`，而当前 Host 合同已移除该匿名 UUID 路由，因此 HTTP 下载失败。文件保留验收已改为固定的 Admin JWT + tenant 受保护资源读取，分别验证升级前、升级后及恢复回滚后的实际字节；缺少用户 JWT 必须 401，不转用 service actor 或匿名路径。文件保留的成功标记移到真实读取与匿名拒绝验证之后。

兼容范围明确：本升级包验证受保护 Admin 文件读取、数据文件保留及声明的 fixture 插件；不声明 legacy `/media` presign 链接可用。旧 Admin presign 返回路径与当前路由的不一致仍需单独对齐，不恢复已移除的匿名路径，也不使用该链接作为本次数据保留验收。该限制写入 manifest 的 `compatibility.post_upgrade_actions`，服务调用方应使用当前 Host transfer 合同。Python 18 项测试通过，待完成真实恢复回滚和发布。

第八轮 [#38144276524](https://github.com/ArtisanCloud/PowerX/actions/runs/38144276524)，目标 SHA `0311d7f20c24f77b0f5965a64cb57ce95e83b91d`：受保护文件读取实际返回 200、内容一致；匿名读取 401。18/19 通过，已进入恢复回滚并恢复数据库，宿主 `shutil.rmtree('data/runtime')` 删除 bind mount 根目录时因父目录属于 Docker root 发生 `PermissionError`。恢复已改为保留现有挂载根目录，只清理并复制内部内容，不改父目录权限；非 CI namespace、缺失挂载和非法目标仍拒绝。新增真实不可写父目录/挂载 inode 保留测试，Python 20 项通过，继续完整回滚验收。

只声明本地文件存储、同 Core 数据库/关闭的向量拓扑、测试进程插件范围。其他存储或业务插件需要额外验收；XDocker 必须按 `compatibility.validated_scope` 判断。

可以审阅和提交本分支，等待 CI 门禁通过后交付发布包。工作区二进制与仅构建成功的镜像不得当作远程自动升级已验收制品。

## 调试清理

本轮只使用短时 CLI、编译/测试进程和独立测试数据库，没有启动 Backend/Web/Redis/MinIO 服务。原用户进程未关闭或重启。交付前删除临时原生源码、二进制、私有配置及专用测试库；验收 JSON 不含凭证原值。
