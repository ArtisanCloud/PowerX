# 原任务命令的启动身份与人工排空确认

2026-11-10，T158 独立工程切片。记录的是原任务某一次本机前台命令组的退出证明，不是业务成功、异地提交、采集完成或删除许可。

## 身份与核对边界

新执行的命令协议为 2：fork 后、leader 尚未 Wait/reap 时采集内核 PID／PGID／UID／启动标记、系统启动摘要和命名空间摘要。Linux 使用 /proc 的 starttime、boot_id、PID／mount namespace；Darwin 使用 kern.proc.pid 和 kern.boottime。无参数、环境、DSN、令牌或密钥进入记录。

原协议 1 的 idle／drained 继续作为原执行器已完成退出检查的记录使用；新协议缺少有效身份、旧 running、prepared 无 PID、其他主机、其他 boot／namespace、损坏或缺失记录都不能补造人工证明。身份捕获失败会结束所属命令组并保留未知，不降级为按 PID 认证。

Root 只读核对必须拿到原命令、原策略／GC／迁移（若有）锁，原任务／执行／命令匹配，策略明确暂停归档和远端清理。读取当前独立恢复代次，检查同一次启动内的原 leader 和原进程组。只有完整启动身份、leader absent **且** group absent 才 eligible。PID 存在但启动身份不同视为 identity_mismatch；权限不足或观察不完整仍未知。核对不发送终止信号，不自动等待到“看起来通过”，也不根据进程缺失单独放行。短期协调锁可能建立锁文件，但不写任务、命令、证明或对象。

任务命令仍限原任务身份。后续 T158 已通过[探测范围确认扩展](storage-archive-probe-confirmation.md)接入协议 3 的无 job 目标探测；旧探测无绑定仅只读显示 probe_scope_missing，继续保护，不能挪用任意任务替它确认。

## Root API

部署前缀默认 `/api/v1/admin/ops/backup/archives`；支持现有自定义前缀。

| 方法与相对路径 | 行为 |
| --- | --- |
| GET `/commands/:record_id/review` | 原记录、当前身份与锁核对，no-store |
| POST `/commands/:record_id/confirmations` | 明确记录人工本机证明，no-store |
| GET `/command-confirmation-requests/:request_id` | 原请求只读回执，no-store |

真实平台 Root 用户会话才可访问；服务／插件／API Key 不能代替。三条路由是 root_runtime_control 运维控制，不发布租户或 Framework 能力，不扩大 RBAC、不 seed 日常库。未修改业务调用的存储准入或 schema readiness。

POST 固定 DTO：

```json
{
  "request_id": "新的请求 UUID",
  "expected_version": "本次核对返回的 SHA256",
  "execution_id": "原执行 UUID",
  "command_id": "原命令 UUID",
  "action": "confirm_local_command_drain",
  "reason": "人工核对原执行和同次启动，记录本机组退出证明",
  "confirm": true
}
```

不接受 PID、信号、命令、路径或任意代理输入。原因至少 5 个 Unicode 字符、最多 1024 UTF-8 字节，首尾无空白、无 NUL／CR；严格 JSON 绑定拒绝未知字段、超大或拼接输入。

## 版本、原子提交与审计

review_version 绑定当前实例／主机、原命令全文、原任务全文、当前暂停策略、独立恢复代次和系统观察。POST 在原锁、配置互斥锁及恢复锁下重查，旧预览、并发执行、策略或恢复变化拒绝。

唯一的正向效果是原子写入私有 `command-confirmations/<command_id>.json`。它同时是不可变终态回执和命令证明，不需要第二个索引或跨文件原子提交。内含原任务／执行／命令、新 request UUID、输入摘要、核对版本、命令与任务摘要、boot／namespace、当前恢复代次、观察事实、人员、原因和时间。原任务和命令文件保持字节不变；不改变 CAS／源／远端／策略／开关。

原请求锁阻止同 UUID 换输入或跨记录并发；全局登记锁确保每条命令只确认一次，并限制 10000 条。重复同 UUID 同输入仅返回原回执，不重做，不刷新当前资格；换输入或新 UUID 再次确认原命令拒绝。

正向证明只对当前命令全文摘要、任务 ID／execution_id、同主机／boot／namespace 和独立恢复代次匹配时派生 drained。原任务后来被明确保留仍可沿用同一执行的本机事实；新命令、新执行、缺失、篡改、回放或恢复不继承。确认后不反复把未来重用的 PID 当成旧进程；当前资格来自不可变证明及身份匹配，普通 GET 不发信号或改文件。

## 拒绝与回复丢失

尚未进入正向证明写入的错误可以明确记录为 `rejected`，保存于 `command-confirmation-rejections/<request_id>.json`，同样保留原输入摘要、人员和原因。它不授予排空资格，也不允许原编号换输入。该目录独立上限 10000。

一旦尝试正向文件写入，任何存储／控制事务确认错误都保持未确认，不用 rejected 覆盖可能已完成的证明。文件持久化与正式控制 DB 不声称跨存储原子事务；错误答复不解除保护。原请求 GET 有界查找不可变正向／拒绝回执，冲突或损坏停止核对；不存在不证明此前未提交。控制库恢复后，明确的拒绝可让用户重新核对并使用新请求；真正未知仍保持原请求，不自动 POST。

浏览器提交前保存最小身份与摘要到本登录 sessionStorage，不含原因正文或凭据。只有同一请求／记录／执行／命令／输入／版本的 confirmed 且 current_record_matches=true，或确切 rejected，才结束待确认。历史 confirmed／404／503／错回执保持锁定。刷新、分区、SSE 断线／重连不自动提交；更换身份中止旧读写，旧结果不进入新会话。

## 页面与运行保护

overview 发布 command_confirmation_version=1；旧 Core 不显示提交入口。归档保护提示提供“核对本机命令”，弹层按“作用与边界 → 核对启动身份 → 人工审核确认”纵向分区，窄屏选择。低频内核字段在 details 中查看；草稿和原意图保持，异常和待确认始终可见。

证明及拒绝目录接入既有 fsnotify／SSE；事件使旧预览失效，保留原因。没有常驻轮询、私有任务队列或新增端口。成功后原采集仍可能 interrupted／interruption_pending，需独立内容核验与明确处置；其他未知命令继续保护。原始运行记录保留 running 不代表当前仍活跃，派生 command_drain_state 才体现匹配证明。

## 验收与剩余门槛

[证据](evidence/storage-archive-command-confirmation-validation-20261110.json)区分：真实父组件 SIGKILL 后孤儿活跃／在测试外部结束后确认；控制 PID 启动标记模拟重用比较（未声称强制内核分配重用）；真实 pg_dump 完成后的 final ack 丢失夹具；正式 PostgreSQL／TaskBus 发布与断控制连接；Root HTTP；真实 Nuxt 代理自有后端的截断回复、坏／历史回执、刷新和 SSE。没有拼接为整个 Core／DB 崩溃验收。

Darwin 原生运行，Linux amd64 仅交叉编译。目标探测、缺少历史启动身份的现场核对、远端接收器、真实 OSS、独立于完整文件系统回退的墓碑、现场旧执行器交接、双环境及 7／14 天证据仍待办。禁止旧执行器混跑；回退先暂停并保留所有证明，不能删台账或按旧 PID 杀进程。本轮未重启日常服务、迁移日常库、部署正式站或操作用户归档对象。
