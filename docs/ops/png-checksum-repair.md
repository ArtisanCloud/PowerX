# 已知 PNG IDAT CRC 修复

独立维护入口只处理 SHA256 为 `4b5c5c92cec3b23e6a294fc0eea43234ef5126c5a64f4c6c531ac8430ab0b844` 的 68 字节、1×1 PNG。原图必须是本地、上传完成的资源，当前标准派生任务必须以 `MEDIA_IMAGE_DECODE_FAILED` 失败，且不存在任何 `preview`、`thumbnail`、`thumbnail_64` 变体。

命令只在内存副本中更正 IDAT CRC，保留压缩像素载荷；通过普通严格解码器后生成三种标准派生图。原图、父资产、失败任务历史和正常读取校验规则保持不变。变体元数据的 `recovery` 记录清单 UUID、修复方法及副本 SHA，不伪造任务完成状态。

## 执行

在 Core 仓库根目录执行，报告使用新路径，父目录需存在：

```sh
make media-png-checksum-scan CONFIG=backend/etc/config.yaml TENANT_UUID=<tenant-uuid> ASSETS=<one-core-asset-uuid> REPORT=<new-plan.json>
make media-png-checksum-apply CONFIG=backend/etc/config.yaml PLAN=<plan.json> REPORT=<new-execution.json> CONFIRM=1
```

`powerx.png_checksum_repair.v1` 清单记录环境指纹、原图 SHA、父资产与失败任务快照指纹、输出 SHA/大小、方法及 seal。扫描使用只读事务；执行重新取得父行锁、验证实际原图及快照，仅创建新文件，读回校验后在一个事务中登记三种变体。不 BootstrapApp，不运行迁移，不创建私有队列、不连接 Redis、不修改 TaskBus 历史。

## 验证与恢复

- 执行报告 `success=true`，三种实际派生对象的 MIME、大小、SHA 与数据库一致，完整 PNG 解码通过，尺寸均为 1×1。
- 原图及父资产、原有任务历史不变。正常派生状态投影在三种变体均可用后显示 `ready`；历史失败记录仍保留。
- 已有任意标准变体时拒绝执行。来源、任务、环境或快照变化时必须重新扫描，不能篡改清单后续跑。
- `MEDIA_PNG_REPAIR_RECOVERY_REQUIRED` 表示已经尝试写入新文件而事务结果待核查。报告保留新对象键和 UUID；先核对数据库及存储，不能无条件删除或重跑。
- 若要撤销，先锁定同租户父行并确认当前三个变体仍是报告中创建的 UUID，制定独立撤销清单。只撤销这些变体的登记，确认无引用后删除对应新文件。父资产、原图和失败任务历史无需恢复，因为本次没有修改它们。

修复后手动刷新；插件历史 Core 引用的读取通道另行验收，不通过修改资源归属或静默切换通道来掩盖该问题。
