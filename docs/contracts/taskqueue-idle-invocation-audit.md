# TaskQueue 空消费的调用审计

## 问题与行为

常驻消费者在队列空闲时也会成功完成 `dequeue`。旧版 Core 对每次空读取写入
`capability_registry_invocation_traces`，并写入、发布一条调用结果通知到
`capability_registry_event_publications`。这些通知无人订阅时成为 `unrouted`，
造成两张系统表持续增长，不能用订单量解释，也不能靠定期删除抵消源头写放大。

Core 现在将经过正式执行、返回空消息列表的读取视为观测：保留调用次数和耗时指标，
不逐次保存调用审计、结果通知或总线发布审计。指标不附带逐次变化的 `trace_id`。
调用响应保留原有 Trace ID 和空消息结果；该空读取的 Trace ID 不保证可查询到持久化
调用记录。一次链路中的其他调用仍正常记录。

## 固定范围

只匹配 `corex.platform` 发布的
`com.corex.runtime.taskqueue.service_consume`，协议 `core_internal`，
显式 `completed`、无错误、无 fallback、无调用级幂等键：

- 唯一请求载荷为 `body`；正式 `taskqueue.v2` 的 `dequeue`，租户为匹配的规范 UUID。
- 执行结果只有 `contract_version: taskqueue.v2` 和非 null 的空 `messages` 列表。
- 原始 RuntimeService 类型化结果和解码后的 JSON 列表均支持；历史审计的省略摘要、
  null、未知版本、附加结果字段均不作为空读取证明。

真实消息领取、enqueue、ack、nack、renew、失败、拒绝、幂等键调用及未知格式继续按
原规则保存审计和通知。调用方不能用 `skip_audit` 或其他标志跳过审计。
RuntimeInvoker 的服务身份、已发布能力、租户登记、实时授权检查和 TaskBus 合同不变；
不修改队列、订单、支付或任务历史数据，不增加数据库迁移或插件安装要求。

## 存量与部署

源代码修复需部署新 Core 才会停止新增空读取记录；不会自行清除历史数据。
200 服务器的临时维护仅按另行获准的七天范围处理可核验的空读取及唯一未投递通知，
先完整备份、恢复校验，再分批事务删除。具体限制见
[临时执行器说明](../../scripts/ops/powerx-transition-guard/README.md)。
临时执行器绑定旧 Core commit；升级时停止写入，须核对原生期限执行回执后再交接。

DELETE 和普通 VACUUM 提供库内复用，不等于操作系统磁盘已经缩小。
物理缩盘需要单独核定保留量、额外空间、WAL 和锁表影响，不能放入分钟监控自动运行。

## 验收

隔离测试验证连续空读取无新增两表记录、无总线发布、计数和耗时指标保留且无 Trace ID
标签；逐项验证失败、真实消息、未知格式仍有持久证据。类型化队列测试覆盖
空闲 → 入队 → 领取 → 完成 → 空闲，核对任务历史和三次非空操作审计保留。

线上验收应先更新 dev Core，比较相同空读取请求与两表增量，并核对正常任务、失败审计、
原生期限执行及临时执行器的升级停止回执。dev 通过后再更新 prod；源码测试不能替代线上验收。
