# L8 - 稳定性与全链路回归

## 目标

验证 Skills 在持续操作和回归测试下稳定，且不破坏既有路径。

## 前置条件

1. L1-L7 已通过。
2. 后端测试环境可运行集成测试。
3. 可访问监控/日志页面。

## UI 详细操作步骤（主流程）

1. 在左侧菜单 `技能库` 连续执行导入/发布/调用（建议持续 10~30 分钟）。
2. 并行观察 Agent 执行事件是否出现中断或异常回落。
3. 在审计/trace 页面确认记录持续完整写入。
4. 在监控页观察错误率、P95、fallback 比例。
5. 回归对比其他协议路径是否出现连带异常。

## UI 通过标准

1. 无阻断级错误。
2. 关键指标在团队阈值内。
3. 审计与 trace 记录连续可查。

## API 留证（可复制）

```bash
set -euo pipefail
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend
mkdir -p .gocache .gomodcache

GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills -run TestSkillNonFuncBaseline_ImportInvokeAudit -count=1

GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills -run TestSkillMatchingScaleAvoidLinearMainPath -count=1

GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go test ./tests/integration/skills \
  -run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope' \
  -count=1

echo "L8 PASS"
```

## 失败排查

1. 集成测试波动：固定测试环境并重复 3 次确认是否偶发。
2. 性能退化：优先排查近期发布变更与依赖服务延迟。
3. trace 缺失：检查日志采集/OTel 链路与权限配置。
