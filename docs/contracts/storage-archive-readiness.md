# T155 真实远端验收的只读前置检查

2026-11-09。用户确定未来使用阿里云 OSS，目前尚未创建／配置；T155 真实目标传输与取回保持待办。本轮交付准备工具与手册，不创建云资源，不把本地 MinIO 或配置检查算成远端验收。

## 工具合同

入口 `scripts/ops/check-storage-archive-readiness.py`，仅 Python 标准库。用户显式提供 Core 地址（包含配置的 API 前缀）、短期 Root 会话和全新报告路径；可选显式目标 UUID。没有登录、配置写入、接管、测试目标、S3 网络、归档、删除、重启或迁移操作。

只访问三个固定 GET：

1. `/admin/ops/backup/archives/target-configuration`
2. `/admin/ops/backup/archives/s3-credentials`
3. `/admin/ops/backup/archives/policy-configuration`

非本机 Core 必须 HTTPS，拒绝 URL 用户信息／query／fragment／异常路径。不跟随 HTTP 重定向、不使用环境代理、不关闭 TLS 证书验证。每次读取受超时和 1 MiB 响应上限约束；拒绝重复 JSON key 和不合预期的数据封装。网络／授权错误不回显响应正文或底层异常。

会话通过 `--token-stdin` 或 `--token-file` 提供，不接受令牌 argv 值；文件必须为当前用户所有、私有、普通、单硬链接且非软链，最大 16 KiB。报告只新建 0600 文件，不覆盖已有路径或经最终软链覆盖。报告不是配置备份；只保留已列出的目标元数据／版本／状态，去掉其他字段；URL 内嵌凭据、query 和 fragment 不输出。会话若被服务端反射到保留元数据中则拒绝写报告。

报告版本 `powerx.archive-readiness/v1`。`configuration_ready=true` 仅表示本次三个 Core 元数据满足所选目标的配置条件：

- 目标、策略均为 database/applied/ready，期望／应用／快照版本为同一正整数，无配置问题。
- 独立凭据存储已配置且 ready，无待轮换；所选引用唯一、sealed/active、正版本。
- 目标 UUID 由使用者明确选中，不默认取首个；目标为 HTTPS S3 且不使用显式回环地址（不证明实际独立性），bucket／prefix／region／版本已登记，当前执行节点验证有效。
- 阿里云标准域名若误填原生 `oss-*` 而非 S3 兼容入口，或打开路径样式，给出专用待核对项。自定义域名、网络可达性、独立长期性、云端协议及权限仍需实测。

不判断普通业务准入，不改变策略状态。`t155_passed`、`authorizes_transfer`、`authorizes_configuration_change`、`provider_protocol_verified`、`independence_and_lifetime_verified`、传输与取回标记始终 false。即使候选配置就绪，也不是执行许可或 T155 完成。

退出码：0 为候选配置就绪；3 为前置条件未就绪或来源不可用；2 为输入／会话文件／报告输出错误。3 不是云服务失败的证据，必须阅读 issues，区分未配置、未接管、版本未发布、凭据未就绪与网络读取失败。

## 本轮现场观察

本机 Core `http://127.0.0.1:8077/api/v1` 的三个 GET 均 200。目标和策略为 legacy/ready，目标 0 个、启用归档策略 0 个；独立凭据存储 configured=false、ready=false。这里的 legacy 是未接管状态，不证明磁盘满、权限不足或需要补数据库迁移。真实 CLI 返回 3，并列出具体待办，没有开启归档。

[验证回执](evidence/storage-archive-readiness-validation-20261109.json)分别记录源码、临时 HTTP／CLI 边界测试、真实 Core 只读观察及资源收尾。真实 OSS 目标、云端协议、版本精确取回和长期运行均未执行；T155 不勾选。

## 阿里云 OSS 兼容边界

当前 PowerX 走本仓库的 S3 客户端。阿里云文档给出的 S3 兼容入口格式为 `https://s3.oss-{region}.aliyuncs.com`，例如 region 为 `cn-hangzhou`；请求使用虚拟托管方式，不能把 MinIO 的路径样式照搬到 OSS。账户的默认域名／CNAME 和证书要求也须按实际地域及官方现行规则核对。[官方 SDK 接入说明](https://www.alibabacloud.com/help/en/oss/developer-reference/use-aws-sdks-to-access-oss)、[S3 兼容差异](https://help.aliyun.com/zh/oss/developer-reference/compatibility-with-amazon-s3)。

归档代码还依赖桶版本状态、私有 ACL／policy、生命周期、对象版本清单、PUT 版本返回、指定版本完整读取及精确版本删除。官方列出的 S3 基础 API 支持不能替代这些具体行为验证；未列出也不在本轮直接断言不支持。OSS 真实能力未测，不能仅填写 endpoint 就承诺完全兼容，更不能为了连接成功关闭版本／公开访问／引用保护。详见[接入与验收清单](../guides/deploy/027-monitor-center/usecase-us9-oss-readiness.md)。

## 后续顺序

T155 在 OSS 创建与 UI 目标／凭据引用就绪后恢复：固定隔离前缀及自有源 → 正式执行节点验证 → 归档和完整读取 → 仅自有隔离源消失后的精确取回 → 收尾与不可变回执。禁止用业务源删除代替测试，也不顺带接管旧执行器。

OSS 准备前，可独立推进 T158 的联合故障回归：已接纳归档在远端断连／凭据失效／控制数据库不可用时，保留原 UUID 与本地内容，不能显示远端成功或产生安全删除资格；通过自有协议服务／数据库及实际 UI 反馈验收。它仍不替代 T155 云端验收。旧执行器现场交接、外部密钥、整站恢复、双环境及 7/14 天证据继续待办。
