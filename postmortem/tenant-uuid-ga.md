# Tenant UUID-only GA Postmortem (Template)

> 请在 GA 完成后一周内填写，结合周会材料与 Grafana 截图存档。

## 摘要
- **事件/阶段**：Tenant UUID-only General Availability
- **时间窗口**：填写开始结束时间
- **结果**：是否达成零 legacy header、零回滚、CI 绿灯等目标

## 时间轴
| 时间 | 事件 | 备注 |
| --- | --- | --- |
| YYYY-MM-DD HH:MM | ... | ... |

## 影响评估
- 客户影响（错误码、工单数量）
- 内部影响（回滚、热补丁、延迟）
- 指标（`tenant_header_reject_total`、`tenant_uuid_only_request_total` 等）截图链接

## 成功经验（What Went Well）
- 例：Playbook 演练提前暴露脚本权限问题并修复

## 待改进（What Went Wrong / Surprises）
- 例：部分 CLI 测试未覆盖离线导入路径导致临时修复

## 根因分析（Root Causes）
1. ...
2. ...

## Action Items
| # | 描述 | Owner | 优先级 | 截止日期 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 1 | | | | | ☐ |

## 附件
- Grafana Dashboard 链接
- `reports/tenant-uuid-weekly.md` 对应周次
- `tenant_uuid_burndown.json` artifact
