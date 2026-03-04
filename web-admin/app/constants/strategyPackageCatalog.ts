export type SceneKey =
  | "product_specs"
  | "product_compat"
  | "product_selection"
  | "sop"
  | "contract_quote"
  | "research_longdoc"
  | "ledger_table"
  | "support_faq"
  | "support_policy"
  | "support_troubleshooting"
  | "eng_runbook"
  | "eng_incident"
  | "eng_change"
  | "api_reference"
  | "data_dictionary"
  | "sales_enablement"
  | "marketing_promo_rules"
  | "compliance_regulation"
  | "billing_pricing"
  | "meeting_minutes"
  | "project_docs"
  | "ticket_conversations"
  | "onboarding_training"
  | "sql_kg"
  | "custom_expert";

export type IndexPrereqKey =
  | "index.dense"
  | "index.sparse"
  | "index.hier"
  | "index.kg"
  | "index.time_fields"
  | "index.structured_fields";

export type StrategyPackageKey =
  | "A0_acl"
  | "A1_routing"
  | "A2_time_aware"
  | "A_simple"
  | "B_semantic_chunking"
  | "C_context_enriched"
  | "D_doc_augmentation"
  | "E_query_transform"
  | "F_rerank"
  | "G_rse"
  | "H_fusion"
  | "I_hyde"
  | "J_hier"
  | "K_kg"
  | "L_feedback"
  | "M_adaptive"
  | "N_self_rag"
  | "O_crag";

export type StrategyPackage = {
  key: StrategyPackageKey;
  label: string;
  summary: string;
  phase: string;
  coupling: "weak" | "strong";
  dependencies: {
    index: IndexPrereqKey[];
    runtime: string[];
    assets: string[];
  };
  recommendedScenes: SceneKey[];
  recommendedProfileKey: "p0_basic" | "p1_general" | "p2_high_accuracy" | "p3_kg_strong";
};

export const STRATEGY_PACKAGE_ORDER: StrategyPackageKey[] = [
  "A_simple",
  "A0_acl",
  "C_context_enriched",
  "F_rerank",
  "H_fusion",
  "A1_routing",
  "E_query_transform",
  "G_rse",
  "I_hyde",
  "L_feedback",
  "M_adaptive",
  "N_self_rag",
  "B_semantic_chunking",
  "J_hier",
  "D_doc_augmentation",
  "A2_time_aware",
  "K_kg",
  "O_crag",
];

export const SCENE_CATALOG: Record<
  SceneKey,
  { category: string; label: string; description: string }
> = {
  product_specs: {
    category: "产品与商品",
    label: "产品库 / 规格参数查询",
    description: "参数/型号/价格/口径等事实精确型查询。",
  },
  product_compat: {
    category: "产品与商品",
    label: "产品库 / 兼容性与配件关系",
    description: "兼容矩阵/配件关系/约束条件。",
  },
  product_selection: {
    category: "产品与商品",
    label: "产品库 / 选型对比与推荐理由",
    description: "方案对比/选型决策，解释型输出。",
  },
  sop: {
    category: "制度与说明",
    label: "SOP/制度/产品说明",
    description: "流程化、章节化内容，强调可追溯引用。",
  },
  contract_quote: {
    category: "合同与报价",
    label: "合同/报价",
    description: "条款/金额/口径准确率要求高。",
  },
  research_longdoc: {
    category: "研究资料",
    label: "论文/研究/长报告",
    description: "长文结构明显，强调语义边界与章节。",
  },
  ledger_table: {
    category: "表格台账",
    label: "台账/清单（表格）",
    description: "行级检索与字段过滤。",
  },
  support_faq: {
    category: "客服支持",
    label: "客服 / FAQ 与使用说明",
    description: "常见问题与标准解答。",
  },
  support_policy: {
    category: "客服支持",
    label: "客服 / 售后政策与规则",
    description: "政策条款，需可追溯引用。",
  },
  support_troubleshooting: {
    category: "客服支持",
    label: "客服 / 故障现象与排查",
    description: "问题定位与排查流程。",
  },
  eng_runbook: {
    category: "工程运维",
    label: "工程 / Runbook 与标准操作",
    description: "流程化操作说明。",
  },
  eng_incident: {
    category: "工程运维",
    label: "工程 / 故障排查与应急响应",
    description: "多步骤应急手册。",
  },
  eng_change: {
    category: "工程运维",
    label: "工程 / 变更与发布",
    description: "发布策略与变更记录。",
  },
  api_reference: {
    category: "研发文档",
    label: "API/接口文档 / 参数与返回",
    description: "接口字段与返回值精确检索。",
  },
  data_dictionary: {
    category: "研发文档",
    label: "数据字典 / 表结构与字段口径",
    description: "字段与口径说明。",
  },
  sales_enablement: {
    category: "销售与市场",
    label: "销售材料 / 话术与竞品",
    description: "话术、卖点、竞品对比。",
  },
  marketing_promo_rules: {
    category: "销售与市场",
    label: "市场活动 / 促销规则",
    description: "优惠规则与口径。",
  },
  compliance_regulation: {
    category: "合规与政策",
    label: "法规/监管政策 / 口径与约束",
    description: "合规口径与约束。",
  },
  billing_pricing: {
    category: "合规与政策",
    label: "计费/价格规则 / 口径与例外",
    description: "价格与计费规则。",
  },
  meeting_minutes: {
    category: "会议与项目",
    label: "会议纪要 / 决议与行动项",
    description: "摘要与行动项检索。",
  },
  project_docs: {
    category: "会议与项目",
    label: "项目方案 / 交付文档",
    description: "方案类长文与交付文档。",
  },
  ticket_conversations: {
    category: "服务工单",
    label: "工单/聊天记录 / 问题追踪",
    description: "多轮对话与追踪记录。",
  },
  onboarding_training: {
    category: "培训与知识",
    label: "入职/培训资料 / 学习路径",
    description: "学习路径与课程材料。",
  },
  sql_kg: {
    category: "研发文档",
    label: "SQL/配置/依赖关系（KG 强）",
    description: "依赖关系与配置链路。",
  },
  custom_expert: {
    category: "自定义",
    label: "自定义（专家）",
    description: "允许专家自定义组合策略。",
  },
};

export const STRATEGY_PACKAGE_CATALOG: Record<StrategyPackageKey, StrategyPackage> = {
  A0_acl: {
    key: "A0_acl",
    label: "A0 Metadata/ACL 优先过滤",
    summary: "先做租户/权限/元数据过滤，再做召回，保证合规降噪。",
    phase: "Recall / Filter",
    coupling: "weak",
    dependencies: {
      index: [],
      runtime: ["acl_enforcer"],
      assets: ["metadata规范化"],
    },
    recommendedScenes: [
      "product_specs",
      "product_compat",
      "product_selection",
      "sop",
      "contract_quote",
      "research_longdoc",
      "ledger_table",
      "support_faq",
      "support_policy",
      "support_troubleshooting",
      "eng_runbook",
      "eng_incident",
      "eng_change",
      "api_reference",
      "data_dictionary",
      "sales_enablement",
      "marketing_promo_rules",
      "compliance_regulation",
      "billing_pricing",
      "meeting_minutes",
      "project_docs",
      "ticket_conversations",
      "onboarding_training",
      "sql_kg",
      "custom_expert",
    ],
    recommendedProfileKey: "p1_general",
  },
  A1_routing: {
    key: "A1_routing",
    label: "A1 Query Routing（查询路由）",
    summary: "按意图/领域把 query 路由到不同索引通道或空间。",
    phase: "Rewrite / Recall",
    coupling: "weak",
    dependencies: {
      index: [],
      runtime: ["routing_policy"],
      assets: ["领域/路由表"],
    },
    recommendedScenes: [
      "product_specs",
      "support_faq",
      "support_policy",
      "eng_runbook",
      "compliance_regulation",
      "sales_enablement",
      "marketing_promo_rules",
      "project_docs",
      "custom_expert",
    ],
    recommendedProfileKey: "p1_general",
  },
  A2_time_aware: {
    key: "A2_time_aware",
    label: "A2 Time-aware（时间/版本）",
    summary: "对版本/生效时间做过滤或权重衰减。",
    phase: "Recall / Fusion",
    coupling: "strong",
    dependencies: {
      index: ["index.time_fields"],
      runtime: ["versioning_policy"],
      assets: ["时间字段/版本线"],
    },
    recommendedScenes: [
      "contract_quote",
      "compliance_regulation",
      "billing_pricing",
      "marketing_promo_rules",
      "product_specs",
    ],
    recommendedProfileKey: "p2_high_accuracy",
  },
  A_simple: {
    key: "A_simple",
    label: "A Simple RAG（最小闭环）",
    summary: "简单切块 + 向量召回，适合快速验证。",
    phase: "Chunking / Recall",
    coupling: "weak",
    dependencies: {
      index: ["index.dense"],
      runtime: [],
      assets: [],
    },
    recommendedScenes: [
      "sop",
      "support_faq",
      "support_policy",
      "support_troubleshooting",
      "eng_runbook",
      "meeting_minutes",
      "project_docs",
      "onboarding_training",
    ],
    recommendedProfileKey: "p0_basic",
  },
  B_semantic_chunking: {
    key: "B_semantic_chunking",
    label: "B Semantic Chunking（语义切块）",
    summary: "按语义边界切块，适合论文/长报告。",
    phase: "Chunking",
    coupling: "strong",
    dependencies: {
      index: ["index.dense"],
      runtime: [],
      assets: ["语义边界检测"],
    },
    recommendedScenes: ["research_longdoc", "project_docs", "meeting_minutes"],
    recommendedProfileKey: "p1_general",
  },
  C_context_enriched: {
    key: "C_context_enriched",
    label: "C Context Enriched（上下文增强）",
    summary: "召回后扩展同章节邻居/父摘要。",
    phase: "Recall / Compress",
    coupling: "weak",
    dependencies: {
      index: ["index.hier"],
      runtime: [],
      assets: ["章节/层次映射"],
    },
    recommendedScenes: ["sop", "contract_quote", "eng_runbook", "support_policy", "compliance_regulation"],
    recommendedProfileKey: "p1_general",
  },
  D_doc_augmentation: {
    key: "D_doc_augmentation",
    label: "D Doc Augmentation（离线增强）",
    summary: "生成摘要/关键词/实体标签等增强字段。",
    phase: "Indexing",
    coupling: "strong",
    dependencies: {
      index: ["index.structured_fields"],
      runtime: ["offline_pipeline"],
      assets: ["摘要/关键词/实体产物"],
    },
    recommendedScenes: ["contract_quote", "product_specs", "sales_enablement", "marketing_promo_rules", "compliance_regulation"],
    recommendedProfileKey: "p2_high_accuracy",
  },
  E_query_transform: {
    key: "E_query_transform",
    label: "E Query Transformation（查询转换）",
    summary: "同义扩展/结构化抽取/纠错。",
    phase: "Rewrite",
    coupling: "weak",
    dependencies: {
      index: ["index.structured_fields"],
      runtime: ["query_rewrite"],
      assets: ["同义词/规则"],
    },
    recommendedScenes: ["product_specs", "data_dictionary", "api_reference", "billing_pricing"],
    recommendedProfileKey: "p1_general",
  },
  F_rerank: {
    key: "F_rerank",
    label: "F Reranker（重排序）",
    summary: "降低相似候选误命中。",
    phase: "Rerank",
    coupling: "weak",
    dependencies: {
      index: ["index.dense"],
      runtime: ["reranker_model"],
      assets: [],
    },
    recommendedScenes: ["product_specs", "product_compat", "contract_quote", "compliance_regulation", "data_dictionary", "api_reference"],
    recommendedProfileKey: "p1_general",
  },
  G_rse: {
    key: "G_rse",
    label: "G RSE（语义扩展重排）",
    summary: "语义扩展 + 重排，适合术语多的场景。",
    phase: "Rerank",
    coupling: "weak",
    dependencies: {
      index: ["index.dense"],
      runtime: ["reranker_model"],
      assets: ["领域词表/实体库"],
    },
    recommendedScenes: ["product_specs", "product_compat", "data_dictionary", "api_reference", "sql_kg"],
    recommendedProfileKey: "p2_high_accuracy",
  },
  H_fusion: {
    key: "H_fusion",
    label: "H Fusion（融合检索）",
    summary: "dense+sparse(+kg/hier) 多路召回融合。",
    phase: "Fusion",
    coupling: "weak",
    dependencies: {
      index: ["index.dense", "index.sparse"],
      runtime: ["score_normalizer"],
      assets: [],
    },
    recommendedScenes: [
      "product_specs",
      "product_compat",
      "product_selection",
      "sop",
      "contract_quote",
      "research_longdoc",
      "ledger_table",
      "support_faq",
      "support_policy",
      "support_troubleshooting",
      "eng_runbook",
      "eng_incident",
      "eng_change",
      "api_reference",
      "data_dictionary",
      "sales_enablement",
      "marketing_promo_rules",
      "compliance_regulation",
      "billing_pricing",
      "meeting_minutes",
      "project_docs",
      "ticket_conversations",
      "onboarding_training",
      "sql_kg",
    ],
    recommendedProfileKey: "p1_general",
  },
  I_hyde: {
    key: "I_hyde",
    label: "I HyDE（假设文档检索）",
    summary: "生成假设答案后再向量检索。",
    phase: "Rewrite",
    coupling: "weak",
    dependencies: {
      index: ["index.dense"],
      runtime: ["llm_generate"],
      assets: [],
    },
    recommendedScenes: ["research_longdoc", "project_docs", "meeting_minutes"],
    recommendedProfileKey: "p1_general",
  },
  J_hier: {
    key: "J_hier",
    label: "J Hierarchical Indices（层次索引）",
    summary: "先章节/摘要，再下钻 chunk。",
    phase: "Recall / Context",
    coupling: "strong",
    dependencies: {
      index: ["index.hier"],
      runtime: [],
      assets: ["章节/摘要索引"],
    },
    recommendedScenes: ["sop", "research_longdoc", "eng_runbook", "compliance_regulation"],
    recommendedProfileKey: "p1_general",
  },
  K_kg: {
    key: "K_kg",
    label: "K Knowledge Graph（知识图谱）",
    summary: "实体/关系召回与约束。",
    phase: "Recall / Filter / Explain",
    coupling: "strong",
    dependencies: {
      index: ["index.kg"],
      runtime: ["graph_query"],
      assets: ["实体/关系抽取产物"],
    },
    recommendedScenes: ["product_compat", "data_dictionary", "api_reference", "sql_kg", "compliance_regulation"],
    recommendedProfileKey: "p3_kg_strong",
  },
  L_feedback: {
    key: "L_feedback",
    label: "L Feedback Loop（反馈闭环）",
    summary: "标注→再加工→回归评估。",
    phase: "Feedback",
    coupling: "weak",
    dependencies: {
      index: [],
      runtime: ["feedback_workflow"],
      assets: ["feedback_case"],
    },
    recommendedScenes: [
      "product_specs",
      "product_compat",
      "product_selection",
      "sop",
      "contract_quote",
      "research_longdoc",
      "ledger_table",
      "support_faq",
      "support_policy",
      "support_troubleshooting",
      "eng_runbook",
      "eng_incident",
      "eng_change",
      "api_reference",
      "data_dictionary",
      "sales_enablement",
      "marketing_promo_rules",
      "compliance_regulation",
      "billing_pricing",
      "meeting_minutes",
      "project_docs",
      "ticket_conversations",
      "onboarding_training",
      "sql_kg",
      "custom_expert",
    ],
    recommendedProfileKey: "p1_general",
  },
  M_adaptive: {
    key: "M_adaptive",
    label: "M Adaptive RAG（自适应策略）",
    summary: "按置信度/成本动态启用策略。",
    phase: "Routing / Orchestration",
    coupling: "weak",
    dependencies: {
      index: [],
      runtime: ["policy_router"],
      assets: ["可观测指标"],
    },
    recommendedScenes: [
      "product_specs",
      "product_compat",
      "product_selection",
      "sop",
      "contract_quote",
      "research_longdoc",
      "ledger_table",
      "support_faq",
      "support_policy",
      "support_troubleshooting",
      "eng_runbook",
      "eng_incident",
      "eng_change",
      "api_reference",
      "data_dictionary",
      "sales_enablement",
      "marketing_promo_rules",
      "compliance_regulation",
      "billing_pricing",
      "meeting_minutes",
      "project_docs",
      "ticket_conversations",
      "onboarding_training",
      "sql_kg",
      "custom_expert",
    ],
    recommendedProfileKey: "p2_high_accuracy",
  },
  N_self_rag: {
    key: "N_self_rag",
    label: "N Self RAG（自反思回路）",
    summary: "自检证据与冲突，触发二次检索。",
    phase: "Answer",
    coupling: "weak",
    dependencies: {
      index: [],
      runtime: ["consistency_checker"],
      assets: ["证据校验链路"],
    },
    recommendedScenes: ["contract_quote", "compliance_regulation", "billing_pricing"],
    recommendedProfileKey: "p2_high_accuracy",
  },
  O_crag: {
    key: "O_crag",
    label: "O CRAG（纠错）",
    summary: "证据不足/冲突时触发纠错检索。",
    phase: "Answer / Feedback",
    coupling: "strong",
    dependencies: {
      index: ["index.dense", "index.sparse"],
      runtime: ["consistency_checker"],
      assets: [],
    },
    recommendedScenes: ["contract_quote", "compliance_regulation", "billing_pricing", "product_specs"],
    recommendedProfileKey: "p2_high_accuracy",
  },
};

