# Tenant UUID GA 签字记录

请在进入 GA 前由各负责人签字确认。每次更新请同步在 PR 描述与 `#tenant-uuid-migration` 中通知相关人。

| Owner | 职责 | 签字时间 | 状态 | 佐证链接 |
| --- | --- | --- | --- | --- |
| Backend Lead | 代码/中间件改造完成且 CI 全绿 | _待填_ | ☐ |  |
| DB Infra | Schema 校验、`pg_checksums`、drop column 验证完成 | _待填_ | ☐ |  |
| Ops | 观测性 & Playbook 演练通过 | _待填_ | ☐ |  |
| QA | CLI/Web Admin/Playwright/Contract 测试绿灯 | _待填_ | ☐ |  |
| DevRel | Docs & Release Notes 发布完毕 | _待填_ | ☐ |  |

> 说明：  
> - 状态：☐ 待签，☑ 已签。  
> - 佐证链接：可填写 PR、Grafana 截图、报告文件等。  
> - 所有人签字后，请在 `tmp/tenant-id-migration-plan.md` 将 T8.8「签字」条目标记为 ✅。
