## 它能做什么

* `-mode=ensure-parent`
  把 `schema.table`（默认 `public.audit_event`）**设为按 `occurred_at` 的 RANGE 分区父表**（幂等；如果已经是分区表就相当于啥也不做）。
* `-mode=ensure` + `-past` + `-future`
  **预创建按月分区**。例如 `-past=1 -future=2` 会确保：上个月、本月、下个月、下下个月 4 个分区都存在。
* `-mode=retire` + `-retention`
  **删除（DROP）早于保留月数的分区**。例如 `-retention=6` 就保留最近 6 个月，把更早的月分区直接 DROP（秒级）。

可选参数：

* `-schema`：默认 `public`
* `-table`：默认 `audit_event`

> 这个工具内部使用 **Advisory Lock**（`pg_try_advisory_lock`），同一时刻只有一台实例会执行，避免多副本并发清理/创建。

---

## 怎么用（两种方式）

### 方式 A：直接用 `go run`

```bash
# 1) 第一次把父表设置为分区表（幂等）
go run cmd/tools/audit_partitions -mode=ensure-parent

# 2) 预建分区：过去 1 个月 + 未来 2 个月
go run cmd/tools/audit_partitions -mode=ensure -past=1 -future=2

# 3) 清理旧分区：只保留最近 6 个月
go run cmd/tools/audit_partitions -mode=retire -retention=6
```

### 方式 B：配合你写的 Makefile 目标

（你已经把 `AUDIT_TOOL=cmd/tools/audit_partitions` 放进 `pkg/make_files/audit_partition.mk`）

```bash
# 一次性：确保父表是分区表
make audit-partitions-parent

# 预建 1 个月过去 + 2 个月未来
make audit-partitions-ensure PAST=1 FUTURE=2

# 清理旧分区：保留 9 个月
make audit-partitions-retire RETENTION=9
```

---

## 运行时会发生什么

* 工具会读取你的 `config.GetGlobalConfig()`，通过 `database.GetDB(&cfg.Database)` 建立 DB 连接。
* 根据 `-mode` 执行相应操作：

    * `ensure-parent`：执行一次 `ALTER TABLE … PARTITION BY RANGE (occurred_at)`（如果已是分区表，内部会忽略报错，幂等）。
    * `ensure`：计算月份窗口（从 `now - past` 到 `now + future`），逐月执行 `CREATE TABLE IF NOT EXISTS schema.audit_event_YYYY_MM … FOR VALUES FROM ('YYYY-MM-01') TO ('YYYY-MM-01+1mon')`。
    * `retire`：枚举现有分区，凡是**分区下界**早于 `cutoff=本月1号 - retention 月**` 的一律 `DROP TABLE`。
* 使用 Advisory Lock：

    * `ensure` 用 key `43210`，`retire` 用 key `43211`。
      如果没拿到锁，当前实例会**直接跳过**（退出码 0），避免并发改表。
* 出错时打印到 `stderr` 并用非零退出码退出（方便 CI/Makefile 发现失败）。

---

## 预期输出 & 验证

* 初次跑 `ensure-parent`：如果父表不是分区表，会顺利转换（无输出或仅你 echo 的提示）；如果本来就是，依旧返回 0。
* 跑 `ensure`：没有现成分区时会创建新分区；再次运行是幂等（`CREATE TABLE IF NOT EXISTS`），无报错。
* 跑 `retire`：当存在超出保留期的老分区时会被 DROP；否则 0 影响。
* 验证：连接数据库 `\d+ public.audit_event`，应显示 `Partitioned table: RANGE (occurred_at)`；` \d+ public.audit_event_2025_08` 等可看到子表。查询也能自动路由到对应分区。

---

## 常见问题

* **“不要 SQL”做分区可不可以？**
  PostgreSQL 的分区管理只能通过 DDL 完成；GORM 没有这类高阶 API。所以我把 SQL 都封装在 `pkg/corex/audit/partition/manager.go` 里了，业务 & Makefile 不直接写 SQL。

* **锁住了怎么办？**
  如果某次任务异常退出且没释放 advisory lock，Postgres 会在 session 结束时自动释放；而我们的 CLI 是短进程，不会常驻。

* **迁移已有大表怎么办？**
  线上大表建议用“中间表 + 触发器 + 交换/重命名”做在线迁移（或用 pg\_partman）。当前这个工具适合**已经是分区表**或**数据量不大**时直接改造。需要线上平滑迁移的脚本也可以再补一版。

---

## 推荐调度

* 用 **crontab/K8S CronJob** 挂两条：

    * 每天/每周跑 `ensure`（预建未来 2 个月）
    * 每天跑 `retire`（保留 6\~12 个月）

这样你的 `audit_event` 表就会一直：**写入快、查询快、清理快**。
