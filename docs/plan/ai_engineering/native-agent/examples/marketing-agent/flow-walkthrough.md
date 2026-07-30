# 营销知识策展流程推演

## 1. 正式 Workflow 形态

营销知识策展必须由 WorkflowInstance 主导。Skill 只作为 Workflow 节点能力，不能承担长流程状态、审核、发布或回滚。

```text
WorkflowDefinition: marketing_knowledge_capture
  input.capture
  skill.invoke.parse_source
  skill.invoke.extract_marketing_knowledge
  metadata.classify
  knowledge.stage
  decision.gateway.conflict
  human.review
  knowledge.publish
  event.emit
```

Workflow runtime 不可用时，该流程不得启动，也不得降级为 Skill + Task Queue。

## 2. 用户故事

营销总监张三每天上传材料：

- 语音复盘。
- 活动 brief。
- 数据截图。
- 渠道投放报告。
- 客户反馈。

目标是持续迭代“张三个人营销方法论库”，并在审核后把部分内容沉淀到“市场部方法论库”。

## 3. Workflow 处理步骤

### 3.1 采集

用户选择目标：

- AgentInstance：公司营销总监智能体。
- Knowledge Space：张三个人营销方法论库。
- 处理模式：营销经验沉淀。

系统创建：

- Source asset。
- Ingestion job。
- WorkflowInstance。
- Agent trace。

### 3.2 解析

`skill.invoke.parse_source` 根据 source 类型调用不同 Skill：

| Source | Skill |
| --- | --- |
| audio | audio.transcribe |
| image | image.ocr |
| pdf/doc | document.parse |
| table | table.extract |
| free text | text.normalize |

### 3.3 抽取

`skill.invoke.extract_marketing_knowledge` 调用营销抽取 Skill：

```text
marketing.strategy.extract
marketing.review.summarize
knowledge.curate.methodology
```

输出 draft：

- Observation。
- Principle。
- Method。
- SOP。
- Decision Rule。
- Case。
- Evidence。

### 3.4 分类和标签

`metadata.classify` 使用 Metadata Governance：

- taxonomy：战略层、策略层、执行层、复盘层。
- tags：channel、funnel_stage、customer_segment、asset_type。
- dictionary：campaign_type、budget_level、customer_intent_signal。
- resource type：source_audio、case_study、playbook、checklist。

### 3.5 冲突检测

`decision.gateway.conflict` 检查新知识是否和已有知识冲突：

| 冲突类型 | 示例 |
| --- | --- |
| duplicate | 已存在同名 SOP |
| contradiction | 新规则与旧规则判断条件相反 |
| stale | 新方法替代旧方法 |
| scope mismatch | 个人经验试图发布到部门库 |

### 3.6 人工确认

`human.review` 给用户展示：

- 新增知识。
- 建议合并知识。
- 冲突知识。
- 建议废弃旧知识。
- Evidence 链接。

用户确认后才允许进入发布节点。

### 3.7 发布

`knowledge.publish` 发布动作：

- 写入 Knowledge Space。
- 生成版本。
- 更新 embedding。
- 更新知识图谱引用。
- 写 audit。
- 发 event。

## 4. 页面映射

| 页面 | 作用 |
| --- | --- |
| 智能体管理 | 选择 PowerX 内置营销负责人智能体并进入克隆 |
| 创建智能体向导 | 克隆为张三营销总监智能体 |
| 智能体管理 | 管理张三智能体、市场部智能体、Campaign 智能体 |
| 专家知识库工作台 | 上传材料、查看草稿、确认发布 |
| 元数据治理 | 管理营销分类、标签、字典、资源类型 |
| 知识库页面 | 查看已发布知识、版本和引用 |

## 5. 验收点

1. 用户可以选择具体 AgentInstance 和 Knowledge Space。
2. 上传材料后产生 source asset 和 draft knowledge objects。
3. 分类、标签、字典来自 Metadata Governance。
4. 发布前必须人工确认。
5. 个人知识库和部门知识库不会自动混写。
6. 每次发布有版本和审计。
