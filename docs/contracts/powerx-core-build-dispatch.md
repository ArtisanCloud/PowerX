# PowerX Core 构建调度与回执 v1

## 交付边界

本合同复用 .github/workflows/publish-container.yml，只控制 Core 构建、隔离验收和发行。
不执行 SaaS 更新、线上迁移、插件安装或生产恢复。严格的 powerx.upgrade-manifest/v1
不增加字段；请求关联使用独立 [powerx.build-receipt/v1 Schema](powerx-build-receipt.schema.json)。

源码实现和本地受控测试已完成；最终工作流提交以本次发布 PR 的合并 SHA 为准。
尚未在 GitHub 上执行新工作流，本地验证不能替代实际 Actions 和镜像验收。
本机 Docker daemon 尚未用于本轮验证；真实双架构升级、数据保留及恢复需在隔离 CI 验收。
固定工作流提交应取最终包含本修复的 commit，不能将开发前 HEAD 或文件摘要当作已发布提交。

## 1. 正式入口

固定仓库 ArtisanCloud/PowerX，固定工作流路径 .github/workflows/publish-container.yml。
workflow_dispatch 只接受以下三个 inputs，全部必填，没有静默默认值：

| 名称 | 规则 |
|---|---|
| request_id | 8–128 个 ASCII 字符；首字符字母或数字，其余为字母、数字、点、下划线、短横线；大小写敏感 |
| expected_source_sha | 40 位小写十六进制完整 SHA |
| build_mode | verify、build 或 release |

ref 使用 GitHub 调度请求自身的字段，不重复放入 inputs：

| 调度 ref | 通道 | 版本格式 |
|---|---|---|
| develop | development | dev- 加 SHA 前 12 位 |
| main、master | stable | release- 加 SHA 前 12 位 |

拒绝 tag、其他分支、未知/缺失 inputs、未知模式、不完整 SHA 和源码不一致。
调用方不能指定镜像仓库、发行 tag、构建命令、任意 URL、通道或请求头。
顺序为：仓库/工作流身份 → 参数/ref → 实际 checkout、run SHA、预期 SHA 完全相等
→ 请求去重 → 检查 → 镜像写入。调度所得提交已不同于预期时，在写 Registry 前失败。
运行建立后继续使用固定 SHA，后续分支移动不会使其他 job 偷换源码。

请求示意，以下全 a SHA 不可实际使用：

    {
      "ref": "develop",
      "inputs": {
        "request_id": "xdocker-request-0001",
        "expected_source_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "build_mode": "build"
      }
    }

默认分支必须保留 workflow_dispatch 入口；新版定义必须先进入实际调度的受支持 ref，再由 XDocker 启用。
不能向尚未升级的旧工作流发送这些 inputs。首次引入阶段，push 入口固定按 verify 规范化，生成 push- 加 run_id 的请求 ID。
首次合入只运行只读检查和回执，不会登录/写入 Registry，也不会创建 Release。
build/release 只接受显式调度。将来恢复自动发行，须另行冻结分支、条件和开关并补齐验收。

## 2. 模式、权限和产物

| 模式 | 执行 | 外部写入 | 回执可部署 |
|---|---|---|---|
| verify | 输入、部署脚本、请求/回执回归、迁移与授权基线 PostgreSQL 检查 | Actions 证据 artifact；不登录/写 Registry，不创建 Release | false |
| build | verify 加双架构镜像、空库 smoke、双架构旧实例升级/恢复、候选部署包 | 固定 GHCR 仓库下本次 CI 标签及证据；不创建 Release | false |
| release | build 加不可覆盖发行、资产下载回查和最终回执 | 同上，加 GitHub Release | 仅完整发行与证据通过才为 true |

Registry 写权限只给 images job；Release 写权限只给 bundle-release job。
preflight 的 Actions 只读权限用于核对历史运行及请求 artifact。
build 是明确允许写临时 Registry 的候选模式，不等于完全无外部写入。

仅使用两个既有仓库：

- ghcr.io/artisancloud/powerx-backend，标签 ci- 加 run_id、attempt。
- ghcr.io/artisancloud/powerx-web-admin，标签 ci- 加 run_id、attempt。

标签表示候选，不代表发行；不改写旧 sha-* 标签，不全局清理 Registry。
临时镜像保留策略由仓库维护者独立管理，本合同不自动删除可能被引用的 digest。
空库 smoke、双架构升级和部署包统一使用已验证来源标签的固定 digest。
前后端都接收同一 SHA、版本和 build_time；BuildKit provenance 及阶段工具版本进入证据。

## 3. 重复请求、重建与取消

请求指纹为以下规范化 JSON（排序键、无空白、UTF-8）的 SHA-256：
request_id、expected_source_sha、build_mode、完整 refs/heads/... ref。

- 同 ID、同指纹：依据已确认的处理归属记录和终态回执，返回 status=duplicate 与
  duplicate_of.run_id/attempt/input_sha256，不再次构建；run ID 大小不表示实际执行顺序。
- 同 ID、不同指纹：BUILD_REQUEST_ID_CONFLICT。
- 历史请求证据缺失、过期、损坏或不能确认归属：阻断，不自动再次执行。
- 同请求组串行，不取消进行中的旧运行。GitHub concurrency 不是请求账本或无限 FIFO；
  平台仍可能取消排队项，XDocker 必须持久化请求/run 关系并核对最终状态。
- 查重固定工作流 dispatch 历史，最多十页、每页 100；结果不完整或超限时失败，
  不跳过查重。长期请求索引超出此范围时须另行升级合同。
- 真正重建使用新的请求 ID；GitHub Re-run all jobs 保留 run ID、递增 attempt，
  各 attempt 的上下文、镜像、报告和回执分开保存。
- 仅重新运行失败 jobs 不保证上游证据同属新 attempt；缺失时明确失败，
  不混用旧证据。首期使用 Re-run all jobs。

### 处理归属和乱序执行

preflight 在现有同请求串行机制内先确认处理归属，再上传独立的
powerx-request-decision-RUN-ATTEMPT artifact，内容为 request-decision.json。
仅归属上传成功且整个 preflight 成功后，checks 和镜像 job 才能执行。
[处理记录 Schema](powerx-build-decision.schema.json) 使用 powerx.build-decision/v1；
powerx.build-receipt/v1 的字段和 Schema 保持不变，回执通过既有 artifacts 引用该记录及摘要。

- 输入 context 只证明收到参数，不等同于处理归属。
- 首次 attempt 尚未开始任何 job、没有处理记录的排队运行不占用归属；后到的 run 101
  可以先处理，run 100 随后应关联已确认的 101，不再次构建。
- 已开始、已完成或已进入重跑却缺少处理记录/回执时，状态为未确认，阻断重放。
- 处理记录、context、最终回执及文件校验和必须对应同一 run/attempt、指纹、源码和定义。
- 多个独立 owner、duplicate 指向 duplicate、循环、自引用和跨 attempt 冒用均拒绝。
- duplicate 直接指向真正 owner。原 owner 显式重跑时，可创建自己的新 attempt；其他运行
  使用经完整核验的 owner 最新 attempt，若该 attempt 仍排队或证据不全则阻断，不能退回旧结果。
- 缺少新处理记录的旧生产者回执不能作为本轮归属证据，不自动重放其历史请求。

images 在自己的 Runner 显式安装 PyYAML 并验证 smoke/upgrade_smoke 导入；preflight
在自己的 Runner 显式安装 jsonschema。smoke.py 使用 main 入口，导入不触发 Setup、数据库或 Docker 操作。

最终回执 job 使用 always() 尽力执行；Runner 故障、排队取消或强制终止仍可能没有回执。
XDocker 结合 GitHub 最终 conclusion 和阶段 artifact 恢复观察；缺回执或状态不明时
保持不可部署，禁止自动补发。取消不撤销已推送镜像，也不证明 Release 上传已撤销。

## 4. 不可覆盖发行

保留既有 docker-dev-SHA12 / docker-release-SHA12 tag 及 v1 源码发行身份。
发行 job 按通道和 SHA12 串行，核对 Git tag 指向完整目标提交。

1. 先创建草稿 Release，冻结完整源码、通道、四份资产的摘要与大小。
2. 已存在资产全部下载回查；差异立即拒绝，不覆盖、不删除。
3. 仅为同一冻结候选的草稿补齐缺失资产；全部回查后才正式发布。
4. 已发布且四份资产完全一致时，只读关联原发行，不再次上传。
5. 已发布但缺资产、未知资产、错误 tag/source 或摘要变化均明确失败。

同源码重建可能改变镜像、报告、manifest 或部署包摘要。v1 不承载多个正式发行变体，
新尝试证据保留，但遇到原发行摘要冲突时拒绝覆盖。若需要同源码多次正式发行，
应另行升级发行身份与消费者合同。上传/发布响应丢失表示结果待确认；
同一候选可回查、恢复草稿缺失上传，不同候选不能自动接管已有草稿。

## 5. 回执与 XDocker 接入

最终 artifact：powerx-build-receipt- 加 run_id、attempt。
内容为 powerx-build-receipt.json 和同名 .sha256。
request、decision、phase、pair、report、bundle、publication artifact 同样隔离 run/attempt。

回执包含输入和指纹、预期/实际 SHA、模式和通道；工作流路径/ref/SHA/文件摘要；
run ID 和 attempt；阶段结果、受控错误码、工具版本、Action refs、构建 metadata/provenance；
成对 index/platform digest；manifest、报告、部署包、校验文件的 artifact 引用/摘要/大小；
Release ID、tag、资产 ID/摘要、回查结果和 deployable。

XDocker 对齐步骤：

1. 登记最终工作流提交、受支持 ref、输入和回执 Schema；保留原 manifest 验证。
2. 请求先持久化，再调用固定 dispatch API，保存返回的 run ID。只确认受理却没有
   run ID 时进入待确认，查找正式回执，不重新 dispatch。
3. 不能仅凭相同 SHA、run-name 或绿色状态认领。核对 request_id、全部规范化输入、
   指纹、可信工作流版本、run/attempt 以及 run.head_sha = expected = actual。
4. 下载后校验来源、Schema、文件摘要、引用归属和跨字段关系；
   Schema 通过本身不证明输入指纹、源码一致或真实性。
5. duplicate 只关联原运行，不能把本次 duplicate 回执直接当作可部署制品。
6. 只有 release、完整双架构、报告/manifest/digest/校验文件一致、Release 回查通过，
   且 GitHub run 完整成功后，才进入后续制品准备。其他状态不转 Ready。

原有消费者使用的 Build backend、Build web admin、空库 smoke、双架构升级和 bundle
发布步骤名称保持。新回执适配由 XDocker 单独完成，Core 不修改其 Ready 标志，
不放宽现有 head_sha、通道和升级证据检查。

### 模式的必需 jobs 和允许跳过项

| 结果/模式 | 必需 jobs | 允许跳过 |
|---|---|---|
| verify（包括 push） | preflight、checks、build-receipt | images、installed-upgrade、upgrade-metadata、bundle-release |
| build | preflight、checks、images、installed-upgrade、upgrade-metadata、build-receipt | bundle-release |
| release | 上述全部，加 bundle-release | 无业务门禁可跳过 |
| duplicate | preflight、build-receipt，且已核验原处理归属 | checks、images、installed-upgrade、upgrade-metadata、bundle-release |

build/release 的 installed-upgrade 必须同时包含 linux/amd64 与 linux/arm64，两个平台均需
完成原有旧实例升级、数据保留和 restore_rollback。任一必需 job 失败、取消或缺证据都不可部署。
首次真实联调只运行 verify 和 build，各用不同 request_id、同一完整源码 SHA；不运行 release。

## 6. 样例与验收

[回执样例目录](examples/powerx-build-receipt-v1/) 全部为虚构 fixture，使用全 a SHA、
示例 run ID 和虚构产物，**不能作为真实 CI 或可部署证据**。

- verify.json、build.json、release.json：三种模式的结构有效样例。
- rejected-input.json、cancelled.json：拒绝、取消的有效回执。
- invalid-*.json：Schema 必须拒绝。
- semantic-sha-mismatch.json：结构可通过，但跨字段 SHA/指纹校验必须拒绝。

本地验证命令：

    PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/powerx/container -p 'test_*.py'
    GOWORK=off go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 -shellcheck= -pyflakes= .github/workflows/publish-container.yml

真实隔离 CI 仍需验证：verify 无 Registry/Release 写入；错误 SHA/模式提前拒绝；
build 双架构升级、数据保留、恢复全通过但不创建 Release；重复请求关联；取消/上传中断；
同源码重建不同 digest 拒绝覆盖；release 回执与公开资产一致。
restore_required 继续要求匹配数据库、配置原密钥、运行文件及适用外部存储恢复，
不能只切旧镜像。

## 实现映射

| 文件 | 责任 |
|---|---|
| .github/workflows/publish-container.yml | 权限分阶段、固定源码、隔离资源、双架构门禁、回执 |
| deploy/powerx/container/build_contract.py | 严格输入、请求关联、阶段证据、digest 环境、回执语义 |
| deploy/powerx/container/release_publish.py | 草稿准备、不可覆盖上传、tag/资产回查 |
| deploy/powerx/container/upgrade_release.py | 既有升级合同；正式流水线要求双平台报告 |
| docs/contracts/powerx-build-receipt.schema.json | 独立、严格 v1 回执结构 |


## 7. 本轮补齐与远端交付顺序（2026-11-12）

本轮修复 images 独立安装 PyYAML、乱序归属判定及 push 默认 verify；没有修改
powerx.build-receipt/v1 的 Schema。新增独立 decision Schema，明确收到输入与已处理归属的区别。
本地隔离 venv 验证先复现缺 yaml，再仅安装 PyYAML 6.0.3，成功导入 smoke/upgrade_smoke；
导入不调用 Docker、Setup 或数据库。64 项本地测试、actionlint 和工作流 Shell 语法检查通过。

只读远端核对：默认分支 release/v1.0.0 已有 dispatch 入口；develop 是合同允许的目标 ref。
当前工作分支 024-ai-engineering-skills 不能冒充已接入版本。远端 develop 已领先本轮开发基线，
发布前须重新核对最新头部，只合入本轮限定文件，不能强推或带入其他任务的改动。

经明确批准发布后，交回实际 develop 完整 SHA、该提交内工作流 SHA256、回执 Schema 摘要，
以及 verify/build 的真实 run ID、attempt、request_id、回执 artifact、成对镜像 digest、
amd64/arm64 升级报告。两次模式使用不同 ID，绑定同一完整 SHA；不执行 release，不更新 SaaS。
缺少发布或真实 CI 证据时，第 4、5 项保持未完成，不能用本地数字替代。
