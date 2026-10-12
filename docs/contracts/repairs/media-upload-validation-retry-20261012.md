# 媒体上传类型校验与失败重试修复

## 已确认的服务器问题

测试环境 `powerx_dev` 的两次上传均正确落盘，但完成确认被旧 MIME 字符串比较误拒绝：

| 文件 | 时间（北京时间） | 结果与 Trace |
| --- | --- | --- |
| README.md，1156 字节 | 2026-10-12 09:45:43 | 422，`d4b9826c-d4db-4dea-aabf-931a73086ca9` |
| 全品类大货价格表.xlsx，11866 字节 | 2026-10-12 09:50:49 | 422，`9a0ed8da-079d-4c39-bd09-040da5b9c3fd` |
| 再次选择 README.md | 2026-10-12 15:10:48 | 创建返回旧资产 `failed`，AI Craft 在上传前拒绝并报 409；创建 Trace `c76c357b-60d3-45c0-87fb-2a8be874eccc` |

两份实际文件的大小、SHA256 与资产声明一致。XLSX 包含合法的内容类型与 Excel 工作簿。

## 修复行为

1. 原文件与 Variant 共用真实内容校验。忽略 MIME 的 charset 参数差异；Markdown / CSV 接受真实普通文本。图片冒充文本仍拒绝。
2. XLSX / DOCX / PPTX 检查 ZIP 目录、`[Content_Types].xml` 对应声明与工作簿／文档／演示文稿 XML 的根元素及 namespace，拒绝普通 ZIP、其他 Office 类型、图片或损坏包冒充。
3. Office 检查限制压缩文件 64 MiB、4096 个条目、中央目录 4 MiB、条目声明的总展开大小 512 MiB、内容类型 XML 256 KiB、主 XML 4 MiB。临时文件权限 0600，检查结束即删除；不展开 ZIP 文件到目录。
4. 大小和实际 SHA256 校验保留。S3 / MinIO 与本地驱动使用同一真实字节判定，不依赖对象的 Content-Type 元数据。
5. 相同文件重复创建时，失败或已过期上传在事务行锁内恢复为 `pending_upload`。必须与原声明的大小、MIME、SHA256 一致，资产 UUID、归属、业务状态及其他元数据保留；已就绪资产继续复用。
6. 原文件 PUT、重试、完成确认共享行锁。PUT 持锁后再次检查授权快照版本；旧 ticket 或已通过前置授权的旧请求不能覆盖新一轮上传。Variant 沿用父资产、子资源的锁顺序。
7. 新一轮版本递增、旧 ticket 失效，重新签发 ticket。原文件 PUT 与 Variant PUT 均单次消费；重试后的 Variant 可覆盖之前失败尝试留下的对象，再重新验证真实内容。
8. 校验失败记录结构化 `reason`、阶段、租户、资产／Variant、声明与检测 MIME、大小及哈希是否匹配；不记录 ticket、凭据、文件内容或哈希原文。资产元数据保留最近 8 次校验失败和最近重试信息，重试另外发出审计事件。

## 如何激活与重试

本次修改位于 PowerX Core 当前分支；通过正常提交、编译及部署更新底座。没有新增表或字段，**此次媒体修复无需额外 migrate 或权限 seed**。

测试环境更新发布目录及软链接后，由部署负责人重启：

```bash
sudo systemctl restart powerx-dev-backend
```

启动成功后，在 AI Craft 页面重新选择之前失败的原文件。现有调用流程仍是：

`create → pending_upload → 新 ticket → PUT → complete-upload → ready`

Core 会自动接管旧失败资产的重试，无需修改数据库状态或删除业务记录。Framework / AI Craft 使用现有 Host 合同即可，本次未新增接口、DTO 字段或权限。

先验证 README 和 Excel 在测试环境成功上传、保存附件、下载内容一致，再更新正式环境并由部署负责人重启 `powerx-backend`。本轮未更新远程程序，也未重启日常服务；服务器页面验收仍待部署后完成。

## 验证记录

- 原文件和 Variant 的本地文件系统上传流程通过；大小／哈希不符、伪造类型、损坏 Office、超量 ZIP 条目和 XML 超限被拒绝。
- 失败资产重试保留诊断，旧 ticket 与旧授权快照拒绝；重复创建并发时仅推进一个版本；已就绪资产保持原版本。
- 原生 PostgreSQL 独立 schema 验证真实行锁、元数据持久化、租户隔离、原文件／Variant 重试及下载字节一致，schema 验收后删除。
- 从服务器只读下载上述实际 XLSX 和 README，使用新校验均通过；临时测试文件验收后删除。
- 针对性 `go test -race`、`go vet` 和 Core 构建通过。
- 仓库 `make unit-test` 缺少 `backend/test-silent.sh`。直接 `go test ./...` 仍有既有失败，包括旧媒体路由断言、Event Fabric / Skills 测试桩接口缺失及 Workflow / IAM 合同测试；未将全量测试标记通过。

## 代码位置

- `backend/internal/service/media/upload_validation.go`：真实内容及 Office 格式校验、失败日志。
- `backend/internal/service/media/upload_retry.go`：重试约束、失败历史与审计。
- `backend/internal/service/media/service.go`：原文件状态流转。
- `backend/internal/service/media/variant_transfer.go`：Variant 上传、完成确认。
- `backend/pkg/corex/db/persistence/repository/media/asset_upload.go`：原文件事务与行锁。
- `backend/pkg/corex/db/persistence/repository/media/variant_transfer.go`：Variant 状态和诊断元数据持久化。
- `specs/001-media-storage/contracts/http-openapi.yaml`：重复创建、完成确认与错误合同说明。
