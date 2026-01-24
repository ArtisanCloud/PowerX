<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, resolveComponent, watch } from "vue";
import { useKnowledgeSpaces, type KnowledgeSpaceRecord } from "~/composables/useKnowledgeSpaces";
import { createQaBridgeClient } from "~/composables/api/services/knowledge-spaces/qaBridgeClient";
import QaBridgeStatusCard from "~/components/knowledge-spaces/QaBridgeStatusCard.vue";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";
import { useEmbeddingGuard, type EmbeddingGuardResult } from "~/composables/useEmbeddingGuard";
import { useConfirm } from "~/composables/useConfirm";
import { useUserStore } from "~/stores/user";
import { resolveTenantUUIDForRequest } from "~/utils/tenant-context";
import { findEnableOcrRecommendation } from "~/utils/knowledge-spaces/recommendations";
import { buildIngestionRemediation, type IngestionRemediation } from "~/utils/knowledge-spaces/ingestionRemediation";
import { useMediaAssetService } from "~/composables/api/services/mediaAssetService";
import { useWSBus } from "~/composables/useWSBus";
import { useFileHash } from "~/composables/useFileHash";
import { SCENE_STRATEGY_CATALOG, type SceneKey } from "~/constants/sceneStrategyCatalog";
import {
	SCENE_CATALOG,
	STRATEGY_PACKAGE_CATALOG,
	STRATEGY_PACKAGE_ORDER,
	type StrategyPackageKey,
} from "~/constants/strategyPackageCatalog";

type SegmentMode = "unit" | "heading" | "clause" | "semantic" | "table_row" | "code_block" | "conversation";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

useHead(() => ({
  title: t("knowledgeSpaces.head.title"),
  meta: [{ name: "description", content: t("knowledgeSpaces.head.description") }],
}));

const api = useKnowledgeSpaces();
const qaClient = createQaBridgeClient();
const knowledgeStore = useKnowledgeSpaceStore();
const userStore = useUserStore();
const { ensureEmbeddingReady } = useEmbeddingGuard();
const { confirm } = useConfirm();
const toast = useToast();
const media = useMediaAssetService();
const { buildStorageKeyFromFile } = useFileHash();
const wsBus = useWSBus();

const spacesLoading = ref(false);
const spacesError = ref<string | null>(null);
const spaces = ref<KnowledgeSpaceRecord[]>([]);
const spacesLoaded = ref(false);
const pendingOpenIngestion = ref(false);
const spaceQuery = ref("");
const systemPanelsOpen = ref(false);
const departmentFilter = ref<string>("all");
const statusFilter = ref<string>("all");
const pagination = reactive({ page: 1, pageSize: 10 });
const pageSizeItems = [
	{ label: "10", value: 10 },
	{ label: "20", value: 20 },
	{ label: "50", value: 50 },
];

const lastSelectedSpaceKey = "px_last_space_id";
const retiringSpaceId = ref<string | null>(null);
const embeddingGuard = ref<EmbeddingGuardResult | null>(null);


const loadSpaces = async () => {
  spacesLoading.value = true;
  spacesError.value = null;
  try {
    const items = await api.listSpaces({ limit: 200 });
    spaces.value = items || [];
    spacesLoaded.value = true;

    const stored = process.client ? localStorage.getItem(lastSelectedSpaceKey) : null;
    const preferred = stored && spaces.value.some((s) => s.spaceId === stored) ? stored : spaces.value[0]?.spaceId;
    if (preferred && !ingestionForm.spaceId) {
      ingestionForm.spaceId = preferred;
    }
  } catch (e: any) {
    spacesError.value = e?.message || "加载空间失败";
    spaces.value = [];
    spacesLoaded.value = true;
  } finally {
    spacesLoading.value = false;
  }
};

const quickActions = computed(() => [
  {
    icon: "i-heroicons-plus-circle",
    title: t("knowledgeSpaces.hero.actions.create"),
    description: t("knowledgeSpaces.hero.actions.createDesc"),
    onClick: goCreateSpace,
    primary: true,
  },
  {
    icon: "i-heroicons-magnifying-glass",
    title: "Playground",
    description: "对比不同 RAG Profile 的检索效果",
    to: "/knowledge-spaces/playground",
  },
  {
    icon: "i-heroicons-book-open",
    title: t("knowledgeSpaces.hero.actions.docs"),
    description: t("knowledgeSpaces.hero.actions.docsDesc"),
    to: "/docs/knowledge-spaces",
  },
]);

const timelinePlaceholders = computed(() => [
	{
		title: t("knowledgeSpaces.timelinePlaceholders.ingestion.title", "暂无入库记录"),
		description: t(
			"knowledgeSpaces.timelinePlaceholders.ingestion.description",
			"当你开始入库后，这里会显示入库作业与结果摘要。",
		),
	},
	{
		title: t("knowledgeSpaces.timelinePlaceholders.fusion.title", "融合未配置"),
		description: t(
			"knowledgeSpaces.timelinePlaceholders.fusion.description",
			"连接多源与融合策略后，这里会显示融合状态与冲突提示。",
		),
	},
	{
		title: t("knowledgeSpaces.timelinePlaceholders.feedback.title", "暂无反馈"),
		description: t(
			"knowledgeSpaces.timelinePlaceholders.feedback.description",
			"提交反馈案例后，这里会显示处理进度与回滚/再加工记录。",
		),
	},
]);

interface QaDashboardStatus {
  latencyMsP95: number;
  citationCoverage: number;
  toolSuccessRate: number;
  degradeCount: number;
  lastAuditId?: string;
  lastUpdatedAt?: string;
}

const qaStatus = ref<QaDashboardStatus>({
  latencyMsP95: 0,
  citationCoverage: 0,
  toolSuccessRate: 0,
  degradeCount: 0,
});

const refreshQaStatus = async () => {
  const tenantUuid = resolveTenantUUIDForRequest();
  if (!tenantUuid) {
    return;
  }
  try {
    const plan = await qaClient.plan({
      tenantUuid,
      intent: "dashboard-health-check",
      domainTags: ["ops"],
      sessionId: "knowledge-dashboard",
      latencyBudgetMs: 2000,
    });
    qaStatus.value = {
      latencyMsP95: plan.latencyBudgetMs ?? 2000,
      citationCoverage: plan.candidateSpaces[0]?.citationCoverage ?? 0,
      toolSuccessRate: plan.tooling.length > 0 ? 0.99 : 0.9,
      degradeCount: plan.degradeCount ?? 0,
      lastAuditId: plan.telemetry?.traceId,
      lastUpdatedAt: plan.telemetry?.recordedAt,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (message.includes("No available OpenAI account")) {
      toast.add({
        color: "warning",
        title: "没有可用的 OpenAI 账号",
        description: "请到「AI 设置」配置可用的 Provider/账号后再重试。",
      });
    } else {
      toast.add({
        color: "error",
        title: t("knowledgeSpaces.qaCard.loadError"),
        description: message,
      });
    }
    console.error(t("knowledgeSpaces.qaCard.loadError"), error);
  }
};

type SegmentOrderKey = "page" | "size" | "segment" | "separator";

const defaultSegmentOrder: SegmentOrderKey[] = ["page", "size", "segment", "separator"];

const ingestionForm = reactive({
	spaceId: "",
	format: "pdf",
	sourceUri: "",
	ingestionProfile: "builtin/default",
	processorProfile: "builtin/default",
	ocrRequired: false,
	maskingProfile: "",
	priority: "normal",
	segmentMode: "unit" as SegmentMode,
	chunkSize: 800,
	chunkOverlap: 120,
	segmentSizePolicy: "target" as "cap" | "target",
	segmentOrder: [...defaultSegmentOrder],
	pagePriority: false,
	anchorHeadingPath: true,
	anchorClauseId: false,
	anchorRowNumber: false,
	anchorSpeaker: false,
	anchorSentenceIndex: false,
});

// 入库来源方式：本地上传 or 远程 URL（需要后端可直接抓取/预签名 URL；暂不在这里收集鉴权信息）
const ingestionSourceMethod = ref<"upload" | "url">("upload");
const selectedFile = ref<File | null>(null);
const ingestionRetainSource = ref(true);
const lastUploadedMediaUUID = ref<string>("");
const ingestionSubmitting = ref(false);
const ingestionError = ref("");
const ingestionRemediation = ref<IngestionRemediation | null>(null);
const ingestionModalOpen = ref(false);
const ingestionAdvancedOpen = ref(false);
const ingestionStep = ref<1 | 2 | 3 | 4>(1);
const ingestionStrategyPackageKey = ref<StrategyPackageKey>("H_fusion");
const derivedIngestionSceneKey = computed<SceneKey>(() => {
	const pkg = STRATEGY_PACKAGE_CATALOG[ingestionStrategyPackageKey.value];
	return (pkg?.recommendedScenes?.[0] as SceneKey | undefined) ?? "custom_expert";
});
const derivedIngestionProfileKey = computed(() => {
	const pkg = STRATEGY_PACKAGE_CATALOG[ingestionStrategyPackageKey.value];
	return pkg?.recommendedProfileKey ?? "p1_general";
});
const mapStrategyPackageToRagPrimary = (key: StrategyPackageKey): string => {
	switch (key) {
		case "A_simple":
		case "A1_routing":
		case "A2_time_aware":
		case "B_semantic_chunking":
		case "C_context_enriched":
		case "D_doc_augmentation":
		case "E_query_transform":
		case "F_rerank":
		case "H_fusion":
		case "J_hier":
		case "K_kg":
		case "L_feedback":
		case "O_crag":
			return key;
		case "A0_acl":
			return "A_simple";
		case "G_rse":
			return "F_rerank";
		case "I_hyde":
			return "H_fusion";
		case "M_adaptive":
			return "H_fusion";
		case "N_self_rag":
			return "O_crag";
		default:
			return key;
	}
};
const derivedIngestionRagPrimary = computed(() => mapStrategyPackageToRagPrimary(ingestionStrategyPackageKey.value));
const ingestionResult = ref<{
	jobId: string;
	status: string;
	retryCount: number;
	errorCode?: string;
	reason?: string;
	chunkTotal: number;
	chunkCoveragePct: number;
	embeddingSuccessPct: number;
	maskingCoveragePct: number;
} | null>(null);
const ingestionHistory = ref<Array<{ jobId: string; status: string; completedAt: string }>>([]);
const ingestionTaskPanelOpen = ref(false);
const ingestionTasks = ref<
	Array<{
		spaceId: string;
		jobId: string;
		status: string;
		sourceLabel: string;
		updatedAt: string;
	}>
>([]);
let ingestionPollTimer: number | null = null;
let unsubscribeCorpusCheck: (() => void) | null = null;
const recentSpaces = computed(() => spaces.value);

const spaceStatusLabel = (status?: string) => {
  switch (String(status || "").toLowerCase()) {
    case "active":
      return t("knowledgeSpaces.spaces.status.active", "可用");
    case "pending_iam":
      return t("knowledgeSpaces.spaces.status.pendingIam", "待权限配置");
    case "retired":
      return t("knowledgeSpaces.spaces.status.retired", "已归档");
    default:
      return status || t("knowledgeSpaces.spaces.status.unknown", "未知");
  }
};

const departmentItems = computed(() => {
	const uniq = Array.from(
		new Set(spaces.value.map((s) => String(s.departmentCode || "").trim()).filter(Boolean)),
	).sort((a, b) => a.localeCompare(b));
	return [
		{ label: t("knowledgeSpaces.spaces.filters.allDepartments", "全部部门"), value: "all" },
		...uniq.map((d) => ({ label: d, value: d })),
	];
});

const statusItems = computed(() => [
	{ label: t("knowledgeSpaces.spaces.filters.allStatus", "全部状态"), value: "all" },
	{ label: spaceStatusLabel("active"), value: "active" },
	{ label: spaceStatusLabel("pending_iam"), value: "pending_iam" },
	{ label: spaceStatusLabel("retired"), value: "retired" },
]);

const strategyPackageItems = computed(() =>
	STRATEGY_PACKAGE_ORDER.map((key) => ({
		label: STRATEGY_PACKAGE_CATALOG[key]?.label || key,
		value: key,
	})),
);

const selectedIngestionPackage = computed(() => STRATEGY_PACKAGE_CATALOG[ingestionStrategyPackageKey.value]);
const segmentModeManuallySet = ref(false);
const segmentSizingManuallySet = ref(false);
const segmentAnchorsManuallySet = ref(false);
const segmentSeparatorsManuallySet = ref(false);
const settingSegmentDefaults = ref(false);

type SeparatorOption = { value: string; label: string };
const SEPARATOR_NONE_VALUE = "__none__";
const SEPARATOR_CUSTOM_VALUE = "__custom__";
const separatorOptions: SeparatorOption[] = [
	// 注意：USelect（reka-ui Select）不允许 item value 为空字符串
	{ value: SEPARATOR_NONE_VALUE, label: "不使用（默认）" },
	{ value: "\n\n", label: "空行（\\n\\n）" },
	{ value: "\n", label: "换行（\\n）" },
	{ value: "。", label: "句号（。）" },
	{ value: "；", label: "分号（；）" },
	{ value: "：", label: "冒号（：）" },
	{ value: "，", label: "逗号（，）" },
	{ value: ".", label: "句号（.）" },
	{ value: ";", label: "分号（;）" },
	{ value: "}", label: "右花括号（}）" },
	{ value: "!", label: "感叹号（!）" },
	{ value: "?", label: "问号（?）" },
	{ value: "！", label: "感叹号（！）" },
	{ value: "？", label: "问号（？）" },
	{ value: SEPARATOR_CUSTOM_VALUE, label: "自定义…" },
];
const separatorSelected = ref<string>(SEPARATOR_NONE_VALUE);
const separatorCustomText = ref("");

	const normalizeSeparatorToken = (raw: string) => {
		const token = String(raw ?? "");
		if (!token) return "";

		// 分隔符可能就是换行本身（`\n`/`\n\n`），不能用 `trim()` 否则会被当成空串。
		const trimmed = token.replace(/^[ \t]+|[ \t]+$/g, "");
		const normalized = trimmed.replaceAll("\\n", "\n").replaceAll("\\t", "\t");

		// 仅当输入只包含空格/制表符时视为无效；保留换行作为合法分隔符。
		if (/^[ \t]*$/.test(normalized)) return "";
		return normalized;
	};

const effectiveSeparators = computed(() => {
	const selected = String(separatorSelected.value || "");
	if (!selected || selected === SEPARATOR_NONE_VALUE) return [];
	if (selected === SEPARATOR_CUSTOM_VALUE) {
		const token = normalizeSeparatorToken(separatorCustomText.value);
		return token ? [token] : [];
	}
	const token = normalizeSeparatorToken(selected);
	return token ? [token] : [];
});

const formatSeparatorLabel = (sep: string) => {
	switch (sep) {
		case "\n\n":
			return "\\n\\n";
		case "\n":
			return "\\n";
		case "\t":
			return "\\t";
		default:
			return sep;
	}
};

const effectiveSeparatorsPreview = computed(() => {
	if (!effectiveSeparators.value.length) return "-";
	return effectiveSeparators.value.map((s) => formatSeparatorLabel(s)).join(" / ");
});

const segmentModeOptions = computed(() => {
	// 根据场景/格式给一组“推荐可选项”
	const format = String(ingestionForm.format || "").toLowerCase();
	const sceneKey = derivedIngestionSceneKey.value;
		const base: Array<{ label: string; value: SegmentMode }> = [
			{ label: "按结构（标题/段落）", value: "heading" },
			{ label: "按语义（句子）", value: "semantic" },
			{ label: "按条款/编号", value: "clause" },
			{ label: "按表格行/记录", value: "table_row" },
			{ label: "按代码/块", value: "code_block" },
			{ label: "按对话轮次", value: "conversation" },
			{ label: "按长度窗口（推荐）", value: "unit" },
		];

	// scene 强约束：让“最常用的模式”排前，避免误选
	const prefer: SegmentMode[] = [];
	if (format === "sql") prefer.push("code_block");
	if (format === "csv" || format === "xlsx" || format === "table") prefer.push("table_row");
	if (sceneKey === "contract_quote" || sceneKey === "compliance_regulation" || sceneKey === "billing_pricing") {
		prefer.push("clause");
	}
	if (sceneKey === "research_longdoc") prefer.push("semantic");
	if (sceneKey === "ticket_conversations") prefer.push("conversation");
	if (sceneKey === "sql_kg") prefer.push("code_block");

	const order = (m: SegmentMode) => {
		const i = prefer.indexOf(m);
		return i >= 0 ? i : 100;
	};
	return [...base].sort((a, b) => order(a.value) - order(b.value));
});

const segmentOrderLabels: Record<SegmentOrderKey, string> = {
	page: "分页（PDF）",
	size: "字数策略",
	segment: "分段模式",
	separator: "分隔符",
};

const segmentOrderHelp: Record<SegmentOrderKey, string> = {
	page: "仅 PDF 生效；决定是否先按页分桶",
	size: "目标/上限两种策略",
	segment: "heading / semantic / clause 等",
	separator: "自定义分隔符的切分优先级",
};

const draggingSegmentKey = ref<SegmentOrderKey | null>(null);

const isSegmentStepEnabled = (key: SegmentOrderKey) =>
	ingestionForm.segmentOrder.includes(key);

const toggleSegmentStep = (key: SegmentOrderKey, enabled: boolean) => {
	const order = [...ingestionForm.segmentOrder];
	const idx = order.indexOf(key);
	if (enabled) {
		if (idx !== -1) return;
		const sizeIdx = order.indexOf("size");
		if (sizeIdx !== -1) {
			order.splice(sizeIdx + 1, 0, key);
		} else {
			order.push(key);
		}
	} else if (idx !== -1) {
		order.splice(idx, 1);
	}
	ingestionForm.segmentOrder = order;
	if (!enabled && key === "segment") {
		ingestionForm.segmentMode = "unit";
	}
};

const moveSegmentOrder = (from: number, to: number) => {
	const order = [...ingestionForm.segmentOrder];
	if (from < 0 || to < 0 || from >= order.length || to >= order.length) return;
	const [item] = order.splice(from, 1);
	order.splice(to, 0, item);
	ingestionForm.segmentOrder = order;
};

const onSegmentDragStart = (key: SegmentOrderKey) => {
	draggingSegmentKey.value = key;
};

const onSegmentDragOver = (event: DragEvent) => {
	event.preventDefault();
};

const onSegmentDrop = (key: SegmentOrderKey) => {
	const from = ingestionForm.segmentOrder.indexOf(draggingSegmentKey.value as SegmentOrderKey);
	const to = ingestionForm.segmentOrder.indexOf(key);
	if (from === -1 || to === -1 || from === to) {
		draggingSegmentKey.value = null;
		return;
	}
	moveSegmentOrder(from, to);
	draggingSegmentKey.value = null;
};

const segmentModeHint = computed(() => {
		switch (ingestionForm.segmentMode) {
		case "heading":
			return "适合 SOP/说明文档：按标题/段落保留结构锚点。";
		case "clause":
			return "适合合同/规则：按“1.1/1.2/第X条”等条款边界切分。";
		case "semantic":
			return "适合论文/长报告：按句子边界切分，再用长度窗口兜底。";
		case "table_row":
			return "适合台账/清单：按行作为检索单元，便于字段过滤与精确命中。";
		case "code_block":
			return "适合 SQL/配置：按块/段落切分（后续可升级为 AST/对象级）。";
		case "conversation":
			return "适合工单/聊天：按“发言人: 内容”轮次切分。";
			case "unit":
			default:
				return "按长度窗口切分，并在窗口边界优先用分隔符（如换行/句号）对齐，避免截断句子。";
		}
	});

const segmentAnchorHint = computed(() => {
	switch (ingestionForm.segmentMode) {
		case "heading":
			return "当前为 heading：建议启用“标题路径（heading_path）”。";
		case "clause":
			return "当前为 clause：建议启用“条款编号（clause_id）”。";
		case "table_row":
			return "当前为 table_row：建议启用“表格行号（row_number）”。";
		case "conversation":
			return "当前为 conversation：建议启用“发言人（speaker）”。";
		case "semantic":
			return "当前为 semantic：建议启用“句子序号（sentence_idx）”。";
		case "code_block":
		case "unit":
		default:
			return "锚点建议会随分段模式自动勾选，可手动覆盖。";
	}
});

const chunkSizeSourceHint = computed(() => {
	const scene = selectedIngestionScene.value as any;
	const size = scene?.ingestionDefaults?.chunking?.chunkSize;
	const label = scene?.label;
	if (typeof size === "number" && label) {
		return `默认来自场景「${label}」的 chunk_size=${size}（可手动覆盖）。`;
	}
	return "默认来自场景配置（可手动覆盖）。";
});

const resolveEmbeddingLimit = (profile: any) => {
	if (!profile) return 0;
	const defaults = profile.defaults || profile.Defaults || {};
	const cap = profile.capCache || profile.cap_cache || {};
	const keys = [
		"max_input_tokens",
		"max_tokens",
		"context_length",
		"context_window",
		"context_size",
		"max_context_tokens",
	];
	for (const key of keys) {
		const v = defaults?.[key];
		if (typeof v === "number" && Number.isFinite(v) && v > 0) return Math.trunc(v);
		if (typeof v === "string") {
			const n = Number.parseInt(v, 10);
			if (Number.isFinite(n) && n > 0) return n;
		}
	}
	for (const key of keys) {
		const v = cap?.[key];
		if (typeof v === "number" && Number.isFinite(v) && v > 0) return Math.trunc(v);
		if (typeof v === "string") {
			const n = Number.parseInt(v, 10);
			if (Number.isFinite(n) && n > 0) return n;
		}
	}
	return 0;
};

const embeddingMaxInputTokens = computed(() => resolveEmbeddingLimit(embeddingGuard.value?.profile));
const embeddingLimitHint = computed(() => {
	const limit = embeddingMaxInputTokens.value;
	if (limit <= 0) return "";
	return `Embedding 上限 ≈ ${limit}（字符/近似 token），建议 chunkSize 不超过该值。`;
});

const chunkSizePolicyLabel = computed(() =>
	ingestionForm.segmentSizePolicy === "target" ? "目标长度（会合并短段）" : "长度上限（只切超长）"
);

const chunkSizePolicyHelp = computed(() =>
	ingestionForm.segmentSizePolicy === "target"
		? "先把短段合并到接近目标长度，再做窗口切分（更像 Dify/Coze）。"
		: "只在超过上限时切分，不会合并短段（更保守）。"
);

const segmentPreviewOrder = computed(() => {
	const order = ingestionForm.segmentOrder.length ? ingestionForm.segmentOrder : defaultSegmentOrder;
	return order.map((key) => {
		const label = segmentOrderLabels[key] || key;
		if (key === "page") {
			return `${label}${ingestionForm.pagePriority ? "" : "（未启用）"}`;
		}
		if (key === "segment") {
			return `${label}${isSegmentStepEnabled("segment") ? "" : "（已关闭）"}`;
		}
		if (key === "separator") {
			return `${label}${isSegmentStepEnabled("separator") ? "" : "（已关闭）"}`;
		}
		return label;
	});
});

const segmentPreviewMode = computed(() => (isSegmentStepEnabled("segment") ? ingestionForm.segmentMode : "-"));
const segmentPreviewSeparators = computed(() =>
	isSegmentStepEnabled("separator") ? effectiveSeparatorsPreview.value : "-"
);

const showPagePriority = computed(() => String(ingestionForm.format || "").toLowerCase() === "pdf");
const chunkingFlowHint = computed(() => {
	const isPDF = String(ingestionForm.format || "").toLowerCase() === "pdf";
	const pageFirst = isPDF && ingestionForm.pagePriority;
	const modeLabel = String(ingestionForm.segmentMode || "unit");
	const size = Number(ingestionForm.chunkSize || 0);
	const overlap = Number(ingestionForm.chunkOverlap || 0);
	const sizePolicy = ingestionForm.segmentSizePolicy === "target" ? "目标" : "上限";
	const separators = effectiveSeparatorsPreview.value === "-" ? "无" : effectiveSeparatorsPreview.value;
	const anchors = [
		ingestionForm.anchorHeadingPath ? "heading_path" : "",
		ingestionForm.anchorClauseId ? "clause_id" : "",
		ingestionForm.anchorRowNumber ? "row_number" : "",
		ingestionForm.anchorSpeaker ? "speaker" : "",
		ingestionForm.anchorSentenceIndex ? "sentence_idx" : "",
	].filter(Boolean);
	const anchorText = anchors.length ? anchors.join(" / ") : "无";
	const steps: string[] = [];
	const order = ingestionForm.segmentOrder.length ? ingestionForm.segmentOrder : defaultSegmentOrder;
	let stepIndex = 1;
	const windowHint = size > 0 ? "超长再按分隔符优先、窗口长度兜底切分" : "不做长度窗口切分";

	for (const key of order) {
		switch (key) {
			case "page": {
				const pageState = pageFirst ? "已启用" : "未启用";
				const pageAvailability = isPDF ? "仅对 PDF 生效" : "当前格式不支持";
				steps.push(`${stepIndex}. 分页（pagePriority）`);
				steps.push(`- ${pageAvailability}`);
				steps.push(`- 当前: ${pageState}`);
				steps.push("- 先把整篇文档拆成“页级单位”（每页一个大块）");
				steps.push("- 这一步只决定“分桶边界”，不会做最终切块");
				if (!pageFirst) {
					steps.push("- 已跳过（未启用或非 PDF）");
				}
				stepIndex += 1;
				break;
			}
			case "size": {
				steps.push(`${stepIndex}. 字数策略（chunkSize）`);
				steps.push(`- chunk=${size} overlap=${overlap}（${sizePolicy}）`);
				if (size > 0) {
					if (ingestionForm.segmentSizePolicy === "target") {
						steps.push("- 先尽量合并短段到接近目标，再进入后续步骤");
					} else {
						steps.push("- 低于上限可不再细分，超限才进入后续步骤");
					}
				} else {
					steps.push("- chunkSize=0：不做长度上限筛选");
				}
				stepIndex += 1;
				break;
			}
			case "segment": {
				steps.push(`${stepIndex}. 分段模式（segmentMode）`);
				if (!isSegmentStepEnabled("segment")) {
					steps.push("- 已关闭");
				} else {
					steps.push(`- 当前模式: ${modeLabel}`);
					steps.push("- 例如 heading/semantic/clause 等");
				}
				stepIndex += 1;
				break;
			}
			case "separator": {
				steps.push(`${stepIndex}. 分隔符（separators）`);
				if (!isSegmentStepEnabled("separator")) {
					steps.push("- 已关闭");
				} else {
					steps.push(`- 分隔符: ${separators}`);
					steps.push(`- ${windowHint}`);
				}
				stepIndex += 1;
				break;
			}
			default:
				break;
		}
	}

	steps.push(`- anchors: ${anchorText}`);
	return steps.join("\n");
});
const selectedSpaceHasSceneBundle = computed(() => {
	const id = ingestionForm.spaceId;
	if (!id) return false;
	const space = spaces.value.find((s) => s.spaceId === id);
	const flags = space?.featureFlags ?? [];
	return (
		flags.some((f) => f.startsWith("rag.strategy_package:")) ||
		flags.some((f) => f.startsWith("rag.scene:")) ||
		flags.some((f) => f.startsWith("rag.bundle:"))
	);
});

const githubGuideBase = "https://github.com/ArtisanCloud/PowerX/tree/main/docs/guides/knowledge_space/strategy";
const selectedIngestionScene = computed(() => SCENE_STRATEGY_CATALOG.scenes[derivedIngestionSceneKey.value]);
const sceneGuideHref = computed(() => {
	const guide = (selectedIngestionScene.value as any)?.guide || "README.md";
	return `${githubGuideBase}/${guide}`;
});
const overviewGuideHref = computed(() => `${githubGuideBase}/README.md`);

const strategyPackageSceneLabels = computed(() =>
	(selectedIngestionPackage.value?.recommendedScenes ?? []).map((key) => ({
		key,
		label: SCENE_CATALOG[key as SceneKey]?.label || key,
	})),
);
const strategyPackageCouplingLabel = computed(() =>
	selectedIngestionPackage.value?.coupling === "strong" ? "强关联" : "弱关联",
);
const profileLabel = (key: string | undefined) => {
	switch (key) {
		case "p0_basic":
			return "P0 基础";
		case "p1_general":
			return "P1 通用推荐";
		case "p2_high_accuracy":
			return "P2 高准确/合规";
		case "p3_kg_strong":
			return "P3 KG 约束";
		default:
			return key || "-";
	}
};

const strategyPackageDecisionHint = computed(() => {
	switch (ingestionStrategyPackageKey.value) {
		case "B_semantic_chunking":
			return "语义切块策略：会影响下一步分割默认值，适合论文/长报告。";
		case "J_hier":
			return "层次索引策略：优先保留章节锚点，适合手册/SOP。";
		case "D_doc_augmentation":
			return "离线增强策略：依赖稳定块边界与结构化字段。";
		case "A2_time_aware":
			return "时间感知策略：需要时间字段与版本线支持。";
		case "K_kg":
			return "知识图谱策略：需要实体/关系抽取与 KG 索引支持。";
		case "O_crag":
			return "纠错策略：对证据一致性更敏感，适合合同/合规类库。";
		default:
			return "该策略包偏检索/融合策略，分割可独立配置（可在下一步调整）。";
	}
});

const mapProfileToStrategyPackage = (profileKey: string | undefined): StrategyPackageKey => {
	switch (profileKey) {
		case "p0_basic":
			return "A_simple";
		case "p2_high_accuracy":
			return "O_crag";
		case "p3_kg_strong":
			return "K_kg";
		case "p1_general":
		default:
			return "H_fusion";
	}
};

const syncStrategyPackageFromSpace = (spaceId: string) => {
	const space = spaces.value.find((s) => s.spaceId === spaceId);
	const flags = (space?.featureFlags ?? []).map((f) => String(f || "").trim().toLowerCase());
	const rawStrategy = flags.find((f) => f.startsWith("rag.strategy_package:"))?.slice("rag.strategy_package:".length);
	if (rawStrategy && STRATEGY_PACKAGE_CATALOG[rawStrategy as StrategyPackageKey]) {
		settingIngestionStrategy.value = true;
		ingestionStrategyPackageKey.value = rawStrategy as StrategyPackageKey;
		ingestionStrategyAutoHint.value = "";
		ingestionStrategyAutoKey.value = "";
		settingIngestionStrategy.value = false;
		return;
	}
	const rawBundle = flags.find((f) => f.startsWith("rag.bundle:"))?.slice("rag.bundle:".length);
	if (rawBundle) {
		settingIngestionStrategy.value = true;
		ingestionStrategyPackageKey.value = mapProfileToStrategyPackage(rawBundle);
		ingestionStrategyAutoHint.value = "";
		ingestionStrategyAutoKey.value = "";
		settingIngestionStrategy.value = false;
	}
};

watch(
	() => ingestionStrategyPackageKey.value,
	() => {
		if (ingestionModalOpen.value && !settingIngestionStrategy.value) {
			ingestionStrategyManuallySet.value = true;
			ingestionStrategyAutoHint.value = "";
			ingestionStrategyAutoKey.value = "";
		}
	},
);

watch(
	() => ingestionSourceMethod.value,
	(method) => {
		if (method === "upload") {
			// 上传模式：保留由 handleFileChange 写入的 file:// 占位，其它情况清空
			if (!selectedFile.value) ingestionForm.sourceUri = "";
			return;
		}
		selectedFile.value = null;
		ingestionForm.sourceUri = "";
	},
);

const filteredSpaces = computed(() => {
	const q = spaceQuery.value.trim().toLowerCase();
	const dept = departmentFilter.value;
	const st = statusFilter.value;
	return spaces.value.filter((s) => {
		if (dept !== "all" && String(s.departmentCode) !== dept) return false;
		if (st !== "all" && String(s.status) !== st) return false;
		if (!q) return true;
		const hay = `${s.spaceName} ${s.departmentCode} ${s.status} ${s.spaceId}`.toLowerCase();
		return hay.includes(q);
	});
});

watch([spaceQuery, departmentFilter, statusFilter], () => {
	pagination.page = 1;
});

const totalSpaces = computed(() => filteredSpaces.value.length);
const totalPages = computed(() => Math.max(1, Math.ceil(totalSpaces.value / pagination.pageSize)));
watch([totalSpaces, () => pagination.pageSize], () => {
	if (pagination.page > totalPages.value) pagination.page = totalPages.value;
});

const paginatedSpaces = computed(() => {
	const start = (pagination.page - 1) * pagination.pageSize;
	return filteredSpaces.value.slice(start, start + pagination.pageSize);
});

const paginationInfo = computed(() => {
	const total = totalSpaces.value;
	const start = total === 0 ? 0 : (pagination.page - 1) * pagination.pageSize + 1;
	const end = Math.min(pagination.page * pagination.pageSize, total);
	return { start, end, total };
});

const setSpaceAndOpenIngestion = async (spaceId: string) => {
	ingestionForm.spaceId = spaceId;
	await openIngestionModal();
};

	const openPlayground = async (spaceId: string) => {
		if (!(await ensureEmbeddingReady())) return;
		navigateTo({ path: "/knowledge-spaces/playground", query: { spaceId } });
	};

	const openSources = async (spaceId: string) => {
		if (!(await ensureEmbeddingReady())) return;
		navigateTo(`/knowledge-spaces/${spaceId}/sources`);
	};

	const openIngestions = async (spaceId: string) => {
		if (!(await ensureEmbeddingReady())) return;
		navigateTo(`/knowledge-spaces/${spaceId}/ingestions`);
	};

	const openStrategy = async (spaceId: string) => {
		if (!(await ensureEmbeddingReady())) return;
		navigateTo({ path: "/knowledge-spaces/strategy", query: { spaceId } });
	};

const isRetiredSpace = (space: KnowledgeSpaceRecord) =>
	String(space.status || "").toLowerCase() === "retired";

const retireSpace = async (space: KnowledgeSpaceRecord) => {
	if (!space?.spaceId || retiringSpaceId.value) return;
	const ok = await confirm({
		title: "删除空间（软删除）",
		description: `将空间标记为不可用，但会保留入库数据与向量索引。需要继续吗？`,
		confirmLabel: "确认删除",
		cancelLabel: "暂不",
		confirmColor: "error",
		tone: "warning",
		showIcon: true,
	});
	if (!ok) return;
	retiringSpaceId.value = space.spaceId;
	try {
		await api.retireSpace(space.spaceId, { reason: "user_request", dropVectors: false });
		toast.add({
			color: "success",
			title: "空间已删除",
			description: "该空间已进入保留状态，暂不可继续使用。",
		});
		await loadSpaces();
	} catch (error: any) {
		toast.add({
			color: "error",
			title: "删除失败",
			description: error?.message || "删除空间失败，请稍后重试。",
		});
	} finally {
		retiringSpaceId.value = null;
	}
};

const taskSpaceLabel = (spaceId: string) => {
	const space = spaces.value.find((s) => s.spaceId === spaceId);
	if (!space) return spaceId.slice(0, 8);
	return space.spaceName ? `${space.spaceName}（${space.departmentCode}）` : space.spaceId.slice(0, 8);
};

const statusColor = (status?: string) => {
	switch (String(status || "").toLowerCase()) {
		case "active":
			return "success";
		case "pending_iam":
			return "warning";
		case "retired":
			return "neutral";
		default:
			return "neutral";
	}
};

const tableColumns = computed(() => {
	const UBadge = resolveComponent("UBadge");
	const UButton = resolveComponent("UButton");
	return [
		{
			accessorKey: "spaceName",
			header: t("knowledgeSpaces.spaces.table.name", "空间"),
			cell: ({ row }: any) =>
				h("div", { class: "min-w-0" }, [
					h("div", { class: "flex items-center gap-2 min-w-0" }, [
						h(
							"div",
							{ class: "truncate font-semibold text-sm text-[var(--text-primary)]" },
							row.original.spaceName || row.original.spaceId.slice(0, 8),
						),
						h(UBadge as any, { color: "neutral", variant: "soft" }, () => row.original.departmentCode || "-"),
					]),
					h("div", { class: "text-xs text-[var(--text-secondary)] mt-1" }, `ID：${row.original.spaceId.slice(0, 8)}…`),
				]),
		},
		{
			accessorKey: "status",
			header: t("knowledgeSpaces.spaces.table.status", "状态"),
			cell: ({ row }: any) =>
				h(UBadge as any, { color: statusColor(row.original.status), variant: "soft" }, () => spaceStatusLabel(row.original.status)),
		},
		{
			id: "actions",
			header: t("knowledgeSpaces.spaces.table.actions", "操作"),
			cell: ({ row }: any) => {
				const space = row.original as KnowledgeSpaceRecord;
				const actions = [
					h(
						UButton as any,
						{
							size: "xs",
							color: "primary",
							variant: "soft",
							icon: "i-heroicons-arrow-up-tray",
							disabled: isRetiredSpace(space),
							onClick: () => setSpaceAndOpenIngestion(space.spaceId),
						},
						() => t("knowledgeSpaces.spaces.actions.ingest", "入库"),
					),
					h(
						UButton as any,
						{
							size: "xs",
							color: "secondary",
							variant: "soft",
							icon: "i-heroicons-link",
							disabled: isRetiredSpace(space),
							onClick: () => openSources(space.spaceId),
						},
						() => t("knowledgeSpaces.spaces.actions.sources", "连接数据源"),
					),
					h(
						UButton as any,
						{
							size: "xs",
							color: "neutral",
							variant: "soft",
							icon: "i-heroicons-list-bullet",
							disabled: isRetiredSpace(space),
							onClick: () => openIngestions(space.spaceId),
						},
						() => "入库记录",
					),
					h(
						UButton as any,
						{
							size: "xs",
							color: "neutral",
							variant: "soft",
							icon: "i-heroicons-adjustments-horizontal",
							disabled: isRetiredSpace(space),
							onClick: () => openStrategy(space.spaceId),
						},
						() => t("knowledgeSpaces.spaces.actions.strategy", "策略"),
					),
					h(
						UButton as any,
						{
							size: "xs",
							color: "neutral",
							variant: "soft",
							icon: "i-heroicons-magnifying-glass",
							disabled: isRetiredSpace(space),
							onClick: () => openPlayground(space.spaceId),
						},
						() => "Playground",
					),
				];
				if (!isRetiredSpace(space)) {
					actions.push(
						h(
							UButton as any,
							{
								size: "xs",
								color: "error",
								variant: "soft",
								icon: "i-heroicons-trash",
								loading: retiringSpaceId.value === space.spaceId,
								onClick: () => retireSpace(space),
							},
							() => "删除",
						),
					);
				}
				return h("div", { class: "flex flex-wrap gap-2 justify-end" }, actions);
			},
		},
	];
});

const ingestionSpaceItems = computed(() =>
	recentSpaces.value.map((space) => ({
		label: space.spaceName ? `${space.spaceName}（${space.departmentCode}）` : space.spaceId,
		value: space.spaceId,
	})),
);

const hasSpaces = computed(() => ingestionSpaceItems.value.length > 0);
const settingIngestionStrategy = ref(false);
const ingestionStrategyManuallySet = ref(false);
const ingestionStrategyAutoHint = ref<string>("");
const ingestionStrategyAutoKey = ref<StrategyPackageKey | "">("");

watch(
  () => knowledgeStore.lastSpace,
  () => {
    if (knowledgeStore.lastSpace?.spaceId && !ingestionForm.spaceId) {
      ingestionForm.spaceId = knowledgeStore.lastSpace.spaceId;
    }
  },
  { immediate: true },
);

watch(
	() => systemPanelsOpen.value,
	(open) => {
		if (open) refreshQaStatus();
	},
);

watch(
  () => route.query.openIngestion,
  async (flag) => {
    if (!flag) return;
    pendingOpenIngestion.value = true;
    if (spacesLoaded.value) {
      await nextTick();
      const preferredSpaceId = typeof route.query.spaceId === "string" ? route.query.spaceId.trim() : "";
      if (preferredSpaceId && spaces.value.some((s) => s.spaceId === preferredSpaceId)) {
        ingestionForm.spaceId = preferredSpaceId;
      }
      await openIngestionModal();
      pendingOpenIngestion.value = false;
    }
    const nextQuery = { ...route.query } as Record<string, any>;
    delete nextQuery.openIngestion;
    delete nextQuery.spaceId;
		delete nextQuery.ocr;
    router.replace({ path: route.path, query: nextQuery });
  },
  { immediate: true },
);

watch(
  () => spacesLoaded.value,
  async (ready) => {
    if (!ready) return;
    if (!pendingOpenIngestion.value) return;
    await nextTick();
    await openIngestionModal();
    pendingOpenIngestion.value = false;
  },
);

watch(
  () => ingestionForm.spaceId,
  (id) => {
    if (!process.client) return;
    if (id) localStorage.setItem(lastSelectedSpaceKey, id);
		if (id) syncStrategyPackageFromSpace(id);
  },
);

onMounted(async () => {
  await ensureEmbeddingReady();
  await loadSpaces();
  if (process.client) {
    wsBus.connect();
    unsubscribeCorpusCheck = wsBus.subscribe("knowledge.corpus_check.job", (payload) => {
      if (!payload) return;
      const spaceUUID = String(payload.space_uuid || payload.spaceId || "").trim();
      if (!spaceUUID) return;
      if (knowledgeStore.lastCorpusCheckJob && knowledgeStore.lastCorpusCheckJob.space_uuid !== spaceUUID) return;
      knowledgeStore.lastCorpusCheckJob = payload as any;
    });
  }
});

onBeforeUnmount(() => {
  if (unsubscribeCorpusCheck) {
    unsubscribeCorpusCheck();
    unsubscribeCorpusCheck = null;
  }
});

const sourceOptions = computed(() => [
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.pdf", "PDF"), value: "pdf" },
	{ label: "Word (doc/docx)", value: "docx" },
	{ label: "Excel (xlsx)", value: "xlsx" },
	{ label: "CSV", value: "csv" },
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.markdown", "Markdown"), value: "markdown" },
	{ label: "HTML", value: "html" },
	{ label: "SQL", value: "sql" },
	{ label: "Image (OCR)", value: "image" },
	{ label: t("knowledgeSpaces.ingestion.sourceOptions.table", "表格"), value: "table" },
]);

const priorityOptions = computed(() => [
	{ label: t("knowledgeSpaces.ingestion.priorityOptions.normal"), value: "normal" },
	{ label: t("knowledgeSpaces.ingestion.priorityOptions.high"), value: "high" },
]);

const sourceMethodOptions = computed(() => [
	{ label: t("knowledgeSpaces.ingestion.sourceMethod.options.upload"), value: "upload" },
	{ label: t("knowledgeSpaces.ingestion.sourceMethod.options.url"), value: "url" },
]);

const canSubmit = computed(() => {
	if (!ingestionForm.spaceId) return false;
	if (ingestionSourceMethod.value === "upload") return !!selectedFile.value;
	return Boolean(ingestionForm.sourceUri);
});

const canGoNext = computed(() => {
	if (ingestionStep.value === 1) return canSubmit.value;
	if (ingestionStep.value === 2)
		return Boolean(ingestionStrategyPackageKey.value);
	if (ingestionStep.value === 3) return Boolean(ingestionForm.segmentMode);
	return true;
});

const applyAutoStrategySuggestion = (suggestion: { key: StrategyPackageKey; hint: string } | null) => {
	if (!suggestion) return;
	// 如果空间已绑定场景/策略包，则不自动覆盖，只给出“建议”并允许一键应用。
	if (selectedSpaceHasSceneBundle.value || ingestionStrategyManuallySet.value) {
		ingestionStrategyAutoKey.value = suggestion.key;
		ingestionStrategyAutoHint.value = t(
			"knowledgeSpaces.ingestion.sceneSuggestion.keepSpacePreset",
			"当前空间已绑定策略包，未自动切换。建议：{hint}",
			{ hint: suggestion.hint },
		);
		return;
	}
	if (!STRATEGY_PACKAGE_CATALOG[suggestion.key]) return;
	settingIngestionStrategy.value = true;
	ingestionStrategyPackageKey.value = suggestion.key;
	ingestionStrategyAutoKey.value = suggestion.key;
	ingestionStrategyAutoHint.value = suggestion.hint;
	settingIngestionStrategy.value = false;
};

const guessStrategyFromSource = (source: string, format: string): { key: StrategyPackageKey; hint: string } => {
	const name = String(source || "").trim();
	const n = name.toLowerCase();
	const has = (re: RegExp) => re.test(n);

	// 目标：优先按“文件格式”给一个默认场景；若无法更细，则默认通用（SOP/制度/产品说明）。
	if (format === "sql") {
		return { key: "K_kg", hint: "已按文件类型（SQL）默认选择“知识图谱（K）”。" };
	}
	if (format === "csv" || format === "xlsx" || format === "table") {
		return { key: "D_doc_augmentation", hint: "已按文件类型（表格/清单）默认选择“文档增强（D）”。" };
	}
	if (format === "markdown" || format === "html" || format === "docx") {
		return { key: "H_fusion", hint: "已按文件类型（结构化文档）默认选择“融合检索（H）”。" };
	}
	if (format === "image") {
		return { key: "H_fusion", hint: "已按文件类型（图片/OCR）默认选择“融合检索（H）”。如为合同类可切到 CRAG（O）。" };
	}

	if (format === "pdf") {
		// PDF 可根据文件名/URL 做更细的启发式（没有命中则回落通用）
		if (has(/合同|报价|协议|条款|保密|投标|招标/)) {
			return { key: "O_crag", hint: "已按名称关键词（合同/报价/条款）建议“纠错（O）”。" };
		}
		if (has(/论文|研究|白皮书|调研|报告|年报/)) {
			return { key: "B_semantic_chunking", hint: "已按名称关键词（研究/报告）建议“语义切块（B）”。" };
		}
		if (has(/数据字典|schema|字段|表结构/)) {
			return { key: "E_query_transform", hint: "已按名称关键词（数据字典/字段）建议“查询转换（E）”。" };
		}
		if (has(/api|openapi|swagger|接口/)) {
			return { key: "E_query_transform", hint: "已按名称关键词（API/接口）建议“查询转换（E）”。" };
		}
		if (has(/runbook|故障|应急|排查|运维/)) {
			return { key: "H_fusion", hint: "已按名称关键词（故障/应急/排查）建议“融合检索（H）”。" };
		}
		if (has(/变更|发布|回滚/)) {
			return { key: "H_fusion", hint: "已按名称关键词（变更/发布/回滚）建议“融合检索（H）”。" };
		}
		return { key: "H_fusion", hint: "已按文件类型（PDF）默认选择“融合检索（H）”。" };
	}

	return { key: "H_fusion", hint: "未识别到更细预设，默认选择“融合检索（H）”。" };
};

const mapChunkingModeToSegmentMode = (chunkingMode: string, format: string): SegmentMode => {
	const f = String(format || "").toLowerCase();
	if (f === "sql") return "code_block";
	if (f === "csv" || f === "xlsx" || f === "table") return "table_row";
	if (f === "image") return "heading";
	const m = String(chunkingMode || "").toLowerCase();
	if (m.includes("clause")) return "clause";
	if (m.includes("semantic")) return "semantic";
	if (m.includes("table") || m.includes("row")) return "table_row";
	if (m.includes("ast") || m.includes("code") || m.includes("object")) return "code_block";
	// PDF/docx 的文本抽取常见没有明确 heading 标记，默认用“长度窗口 + 分隔符边界”更稳。
	if (f === "pdf" || f === "docx" || f === "markdown" || f === "html") return "unit";
	return "heading";
};

const recommendedSegmentDefaults = computed(() => {
	const scene = selectedIngestionScene.value as any;
	const chunking = scene?.ingestionDefaults?.chunking;
	const baseMode = mapChunkingModeToSegmentMode(chunking?.mode || "", ingestionForm.format);
	const baseSize = typeof chunking?.chunkSize === "number" ? chunking.chunkSize : 800;
	const baseOverlap = typeof chunking?.overlap === "number" ? chunking.overlap : 120;

	// 策略包对分割默认做“有方向的偏置”
	const strategyKey = ingestionStrategyPackageKey.value;
	let mode: SegmentMode = baseMode;
	let chunkSize = baseSize;
	let chunkOverlap = baseOverlap;

	if (strategyKey === "B_semantic_chunking") {
		mode = "semantic";
		chunkSize = Math.max(chunkSize, 1200);
		chunkOverlap = Math.max(chunkOverlap, 200);
	}
	if (strategyKey === "J_hier" || strategyKey === "C_context_enriched") {
		mode = "heading";
	}
	if (strategyKey === "A2_time_aware" || strategyKey === "O_crag") {
		// 证据/时间敏感场景：更偏条款/编号边界
		if (ingestionForm.format === "pdf" || ingestionForm.format === "docx") {
			mode = "clause";
			chunkSize = Math.max(chunkSize, 900);
			chunkOverlap = Math.max(chunkOverlap, 150);
		}
	}
	if (strategyKey === "K_kg") {
		// KG 场景：优先保留结构锚点（SQL→代码块，其它→标题/条款）
		if (ingestionForm.format === "sql") mode = "code_block";
		else if (derivedIngestionProfileKey.value === "p3_kg_strong") mode = baseMode === "clause" ? "clause" : "heading";
	}

	const anchors = {
		anchorHeadingPath: mode === "heading" || strategyKey === "J_hier" || strategyKey === "C_context_enriched",
		anchorClauseId: mode === "clause",
		anchorRowNumber: mode === "table_row",
		anchorSpeaker: mode === "conversation",
		anchorSentenceIndex: mode === "semantic",
	};

	let separators: string[] = [];
	switch (mode) {
		case "table_row":
			separators = [];
			break;
		case "semantic":
			// semantic 已按句子切分，分隔符只作为兜底（默认不强推）
			separators = [];
			break;
		case "clause":
			separators = ["\n\n", "\n", "。", "；", "："];
			break;
		case "code_block":
			separators = ["\n\n", "\n", ";", "}", "。"];
			break;
		case "conversation":
			separators = ["\n\n", "\n", "。", "！", "？"];
			break;
		case "heading":
		default:
			separators = ["\n\n", "\n", "。", "；", "：", ".", "!", "?", "！", "？"];
			break;
	}

	return { mode, chunkSize, chunkOverlap, anchors, separators };
});

watch(
	() =>
		[
			ingestionModalOpen.value,
			ingestionStrategyPackageKey.value,
			derivedIngestionSceneKey.value,
			derivedIngestionProfileKey.value,
			ingestionForm.format,
		] as const,
	() => {
		if (!ingestionModalOpen.value) return;
		const next = recommendedSegmentDefaults.value;
		settingSegmentDefaults.value = true;
		if (!segmentModeManuallySet.value) ingestionForm.segmentMode = next.mode;
		if (!segmentSizingManuallySet.value) {
			ingestionForm.chunkSize = next.chunkSize;
			ingestionForm.chunkOverlap = next.chunkOverlap;
		}
		if (!segmentAnchorsManuallySet.value) {
			ingestionForm.anchorHeadingPath = next.anchors.anchorHeadingPath;
			ingestionForm.anchorClauseId = next.anchors.anchorClauseId;
			ingestionForm.anchorRowNumber = next.anchors.anchorRowNumber;
			ingestionForm.anchorSpeaker = next.anchors.anchorSpeaker;
			ingestionForm.anchorSentenceIndex = next.anchors.anchorSentenceIndex;
		}
		if (!segmentSeparatorsManuallySet.value) {
			separatorSelected.value = next.separators[0] ?? SEPARATOR_NONE_VALUE;
			separatorCustomText.value = "";
		}
		settingSegmentDefaults.value = false;
	},
	{ immediate: true },
);

watch(
	() => ingestionForm.segmentMode,
	() => {
		if (!ingestionModalOpen.value) return;
		if (settingSegmentDefaults.value) return;
		segmentModeManuallySet.value = true;
	},
);

watch(
	() => [ingestionForm.chunkSize, ingestionForm.chunkOverlap, ingestionForm.segmentSizePolicy] as const,
	() => {
		if (!ingestionModalOpen.value) return;
		if (settingSegmentDefaults.value) return;
		segmentSizingManuallySet.value = true;
	},
);

watch(
	() =>
		[
			ingestionForm.anchorHeadingPath,
			ingestionForm.anchorClauseId,
			ingestionForm.anchorRowNumber,
			ingestionForm.anchorSpeaker,
			ingestionForm.anchorSentenceIndex,
		] as const,
	() => {
		if (!ingestionModalOpen.value) return;
		if (settingSegmentDefaults.value) return;
		segmentAnchorsManuallySet.value = true;
	},
);

watch(
	() => [separatorSelected.value, separatorCustomText.value] as const,
	() => {
		if (!ingestionModalOpen.value) return;
		if (settingSegmentDefaults.value) return;
		segmentSeparatorsManuallySet.value = true;
	},
);

const autoSuggestedStrategyLabel = computed(() => {
	const key = ingestionStrategyAutoKey.value;
	if (!key) return "";
	return STRATEGY_PACKAGE_CATALOG[key]?.label || String(key);
});

const canApplyAutoSuggestion = computed(() => {
	const key = ingestionStrategyAutoKey.value;
	if (!key) return false;
	return key !== ingestionStrategyPackageKey.value && Boolean(STRATEGY_PACKAGE_CATALOG[key]);
});

const applyAutoSuggestionNow = () => {
	const key = ingestionStrategyAutoKey.value;
	if (!key) return;
	if (!STRATEGY_PACKAGE_CATALOG[key]) return;
	settingIngestionStrategy.value = true;
	ingestionStrategyPackageKey.value = key;
	settingIngestionStrategy.value = false;
};

const handleFileChange = (event: Event) => {
	const input = event.target as HTMLInputElement;
	const file = input.files?.[0] || null;
	selectedFile.value = file;
	if (file) {
		// 仅用于 UI 展示；真实入库前会上传到 Media 并生成可抓取的 presign URL。
		ingestionForm.sourceUri = `file://${file.name}`;
		const lower = file.name.toLowerCase();
		if (lower.endsWith(".pdf")) ingestionForm.format = "pdf";
		else if (lower.endsWith(".doc") || lower.endsWith(".docx")) ingestionForm.format = "docx";
		else if (lower.endsWith(".xlsx")) ingestionForm.format = "xlsx";
		else if (lower.endsWith(".csv")) ingestionForm.format = "csv";
		else if (lower.endsWith(".md") || lower.endsWith(".markdown")) ingestionForm.format = "markdown";
		else if (lower.endsWith(".html") || lower.endsWith(".htm")) ingestionForm.format = "html";
		else if (lower.endsWith(".sql")) ingestionForm.format = "sql";
		else if (lower.endsWith(".txt")) ingestionForm.format = "markdown";
		applyAutoStrategySuggestion(guessStrategyFromSource(file.name, ingestionForm.format));
	}
};

watch(
	() => ingestionForm.format,
	(format) => {
		if (!ingestionModalOpen.value) return;
		if (String(format || "").toLowerCase() !== "pdf") {
			ingestionForm.pagePriority = false;
		}
		// URL 模式：用户手动选择格式时给出默认场景（无预设则通用）
		if (ingestionSourceMethod.value !== "url") return;
		const source = ingestionForm.sourceUri || "";
		applyAutoStrategySuggestion(guessStrategyFromSource(source, String(format || "")));
	},
);

watch(
	() => ingestionForm.sourceUri,
	(uri) => {
		if (!ingestionModalOpen.value) return;
		if (ingestionSourceMethod.value !== "url") return;
		if (!uri) return;
		applyAutoStrategySuggestion(guessStrategyFromSource(uri, String(ingestionForm.format || "")));
	},
);

const goCreateSpace = async () => {
	if (!(await ensureEmbeddingReady())) return;
	await navigateTo("/knowledge-spaces/create");
};


const openIngestionModal = async () => {
	if (!hasSpaces.value) {
		goCreateSpace();
		return;
	}
	const guard = await ensureEmbeddingReady();
	if (!guard) {
		return;
	}
	embeddingGuard.value = guard;
	ingestionError.value = "";
	ingestionRemediation.value = null;
	ingestionAdvancedOpen.value = false;
	ingestionStep.value = 1;
	ingestionSourceMethod.value = "upload";
	ingestionStrategyManuallySet.value = false;
	segmentModeManuallySet.value = false;
	segmentSizingManuallySet.value = false;
	segmentAnchorsManuallySet.value = false;
	segmentSeparatorsManuallySet.value = false;
	settingSegmentDefaults.value = false;
	ingestionStrategyAutoHint.value = "";
	ingestionStrategyAutoKey.value = "";
	separatorSelected.value = SEPARATOR_NONE_VALUE;
	separatorCustomText.value = "";
	// 默认走“推荐的 builtin/default + default”，高级设置可覆盖。
	ingestionForm.ingestionProfile = "default";
	ingestionForm.processorProfile = "builtin/default";
	ingestionForm.maskingProfile = "";
	ingestionForm.ocrRequired = false;
	ingestionForm.pagePriority = false;
	ingestionForm.segmentOrder = [...defaultSegmentOrder];
	ingestionForm.segmentSizePolicy = "target";
	if (ingestionForm.spaceId) syncStrategyPackageFromSpace(ingestionForm.spaceId);
	ingestionModalOpen.value = true;
};

const closeIngestionModal = () => {
	if (typeof document !== "undefined") {
		(document.activeElement as HTMLElement | null)?.blur?.();
	}
	ingestionModalOpen.value = false;
};

const summarizeSource = (uri: string) => {
	const u = String(uri || "").trim();
	if (!u) return "-";
	try {
		if (u.startsWith("http://") || u.startsWith("https://")) {
			const url = new URL(u);
			const parts = url.pathname.split("/").filter(Boolean);
			return parts[parts.length - 1] || url.hostname;
		}
	} catch {
		// ignore
	}
	const parts = u.split("/").filter(Boolean);
	return parts[parts.length - 1] || u;
};

const isTerminalIngestionStatus = (status: string) => {
	const s = String(status || "").toLowerCase();
	return s === "completed" || s === "failed" || s === "blocked" || s === "paused";
};

const upsertTask = (task: { spaceId: string; jobId: string; status: string; sourceLabel: string; updatedAt: string }) => {
	const idx = ingestionTasks.value.findIndex((t) => t.jobId === task.jobId);
	if (idx >= 0) ingestionTasks.value[idx] = task;
	else ingestionTasks.value.unshift(task);
	if (ingestionTasks.value.length > 20) ingestionTasks.value.length = 20;
};

const refreshIngestionTasks = async () => {
	if (!ingestionTasks.value.length) return;
	const next = [...ingestionTasks.value];
	for (let i = 0; i < next.length; i++) {
		const t = next[i];
		if (isTerminalIngestionStatus(t.status)) continue;
		try {
			const latest = await api.getIngestionJob(t.spaceId, t.jobId);
			next[i] = {
				...t,
				status: latest.status,
				updatedAt: new Date().toISOString(),
			};
		} catch {
			// ignore: best-effort polling
		}
	}
	ingestionTasks.value = next;
};

const startIngestionPolling = () => {
	if (!process.client) return;
	if (ingestionPollTimer != null) return;
	ingestionPollTimer = window.setInterval(() => {
		void refreshIngestionTasks();
	}, 5000);
};

const stopIngestionPolling = () => {
	if (!process.client) return;
	if (ingestionPollTimer == null) return;
	window.clearInterval(ingestionPollTimer);
	ingestionPollTimer = null;
};

const hasRunningIngestionTasks = computed(() => ingestionTasks.value.some((t) => !isTerminalIngestionStatus(t.status)));
const runningIngestionCount = computed(() => ingestionTasks.value.filter((t) => !isTerminalIngestionStatus(t.status)).length);

watch(
	() => [ingestionTaskPanelOpen.value, hasRunningIngestionTasks.value] as const,
	([open, running]) => {
		if (open || running) startIngestionPolling();
		else stopIngestionPolling();
	},
	{ immediate: true },
);

onBeforeUnmount(() => stopIngestionPolling());

const goPrevStep = () => {
	if (ingestionStep.value > 1) ingestionStep.value = (ingestionStep.value - 1) as any;
};

const goNextStep = () => {
	if (!canGoNext.value) return;
	if (ingestionStep.value < 4) ingestionStep.value = (ingestionStep.value + 1) as any;
};

const ocrRecommendation = computed(() =>
	findEnableOcrRecommendation((knowledgeStore.lastCorpusCheckJob as any)?.recommendations),
);

const openOcrPluginMarket = () => {
	const pluginId = ocrRecommendation.value?.pluginId || "com.powerx.plugin.data_forge";
	navigateTo(`/plugins/market?pluginId=${encodeURIComponent(pluginId)}`);
};

const applyOcrSuggestion = () => {
	ingestionForm.ocrRequired = true;
	ingestionAdvancedOpen.value = true;
};

	const buildAbsoluteUrl = (url: string) => {
		const raw = String(url || "").trim();
		if (!raw) return "";
		if (!process.client) return raw;
		if (!raw.startsWith("/")) return raw;
		const cfg = useRuntimeConfig();
		const upstreamOrigin = String(cfg.public?.upstreamOrigin || "").replace(/\/+$/, "");
		const base = upstreamOrigin || location.origin;
		return `${base}${raw}`;
	};

	const uploadSelectedFileToMedia = async (spaceId: string, file: File) => {
		const name = (file.name || "upload").trim();
		const tenantUuid = resolveTenantUUIDForRequest();
		const userScopeId =
			userStore.currentMemberId ||
			userStore.user?.id ||
			"";
		const scopeKey = userScopeId ? `${tenantUuid}:${userScopeId}` : tenantUuid;
		const hashInfo = await buildStorageKeyFromFile(file, scopeKey);
		const tags = [
			"knowledge_space",
		"knowledge_ingestion_source",
		...(ingestionRetainSource.value ? [] : ["ephemeral"]),
	];
	const created = await media.createAsset({
		name,
		uploadMethod: "presign_upload",
		objectKey: hashInfo.uuid || undefined,
		ownerSubjectType: "knowledge_space",
		ownerSubjectId: spaceId,
		tags,
		sizeBytes: file.size,
		mimeType: file.type || undefined,
		metadata: {
			tenantUuid,
			spaceId,
			filename: name,
			sourceType: ingestionForm.format,
			retainSource: ingestionRetainSource.value,
			content_sha256: hashInfo.sha256 || undefined,
		},
	});

	const presign = await media.presign(created.uuid, {
		action: "upload",
		method: "PUT",
		expiresInSeconds: 3600,
			filename: name,
			content_type: file.type || undefined,
		});

		const uploadHeaders = { ...(presign.headers || {}) } as Record<string, string>;
		if (file.type && !uploadHeaders["Content-Type"] && !uploadHeaders["content-type"]) {
			uploadHeaders["Content-Type"] = file.type;
		}
		await media.uploadPresigned({ ...presign, headers: uploadHeaders }, file);

		// best-effort：更新 media 资产状态（是否“保留到媒体库”）
		try {
			// 状态机：draft 不能直接流转到 published（需要 under_review → published）。
			// “保留源文件”默认先进入 under_review，后续可在媒体库中再发布。
			const nextStatus = ingestionRetainSource.value ? "under_review" : "archived";
			await media.updateAsset(created.uuid, {
				businessStatus: nextStatus,
				tags,
			});
		} catch {
			// ignore
		}

	// 入库 Worker 需要一个可抓取的 URL，这里用 presign download 避免鉴权。
	const download = await media.presign(created.uuid, {
		action: "download",
		method: "GET",
		expiresInSeconds: 24 * 3600,
	});
	const sourceUri = buildAbsoluteUrl(download.url);
	if (!sourceUri) throw new Error("预签名下载返回空链接");
	return { sourceUri, mediaUuid: created.uuid };
};

const startCorpusCheckBestEffort = async (spaceId: string) => {
	const lastJob = knowledgeStore.lastCorpusCheckJob as any;
	if (lastJob && lastJob.space_uuid === spaceId) return;
	try {
		const created = await api.startCorpusCheck(spaceId, "");
		knowledgeStore.lastCorpusCheckJob = created as any;
	} catch {
		// ignore: best-effort
	}
};

const submitIngestion = async () => {
	ingestionError.value = "";
	ingestionRemediation.value = null;
	lastUploadedMediaUUID.value = "";
	if (!ingestionForm.spaceId) {
		ingestionError.value = t("knowledgeSpaces.ingestion.errors.missingSpaceId");
		return;
	}
	if (!canSubmit.value) {
		ingestionError.value = t("knowledgeSpaces.ingestion.errors.missingSource");
		return;
	}
	const guard = await ensureEmbeddingReady();
	if (!guard) {
		ingestionError.value = "请先在 AI Settings 配置 embedding 模型并完成测试";
		return;
	}
	embeddingGuard.value = guard;
	const limit = embeddingMaxInputTokens.value;
	if (limit > 0 && ingestionForm.chunkSize > 0 && ingestionForm.chunkSize > limit) {
		ingestionError.value = `当前 embedding 上限≈${limit}，chunkSize 不应超过该值。请调整后再入库。`;
		return;
	}
	ingestionSubmitting.value = true;
	try {
		// 将“策略包”写入空间 feature_flags，保证后续 Playground/策略验证/推荐一致。
		const space = spaces.value.find((s) => s.spaceId === ingestionForm.spaceId);
		const currentFlags = space?.featureFlags ?? [];
		const kept = currentFlags.filter(
			(f) =>
				!f.startsWith("rag.scene:") &&
				!f.startsWith("rag.bundle:") &&
				!f.startsWith("rag.primary:") &&
				!f.startsWith("rag.strategy_package:") &&
				f !== "rag.guided",
		);
		const nextFlags = [
			...kept,
			`rag.strategy_package:${ingestionStrategyPackageKey.value}`,
			`rag.scene:${derivedIngestionSceneKey.value}`,
			`rag.bundle:${derivedIngestionProfileKey.value}`,
			`rag.primary:${derivedIngestionRagPrimary.value}`,
		];
		if (derivedIngestionSceneKey.value === "custom_expert") nextFlags.push("rag.guided");
		try {
			const updated = await api.updateSpace(ingestionForm.spaceId, { featureFlags: nextFlags });
			const idx = spaces.value.findIndex((s) => s.spaceId === ingestionForm.spaceId);
			if (idx >= 0) spaces.value[idx] = updated;
		} catch {
			// ignore: best-effort, do not block ingestion
		}

		let resolvedSource = ingestionForm.sourceUri;
		if (ingestionSourceMethod.value === "upload" && selectedFile.value) {
			const uploaded = await uploadSelectedFileToMedia(ingestionForm.spaceId, selectedFile.value);
			resolvedSource = uploaded.sourceUri;
			lastUploadedMediaUUID.value = uploaded.mediaUuid;
		}
		const payload = {
			format: ingestionForm.format,
			sourceUri: resolvedSource,
			docUuid: lastUploadedMediaUUID.value || undefined,
			ingestionProfile: ingestionForm.ingestionProfile,
			processorProfile: ingestionForm.processorProfile,
			ocrRequired: ingestionForm.ocrRequired,
			maskingProfile: ingestionForm.maskingProfile,
			priority: ingestionForm.priority,
			ragStrategyPackageKey: ingestionStrategyPackageKey.value,
			ragSceneKey: derivedIngestionSceneKey.value,
			ragBundleKey: derivedIngestionProfileKey.value,
			ragPrimary: derivedIngestionRagPrimary.value,
			segmentMode: ingestionForm.segmentMode,
			chunkSize: ingestionForm.chunkSize,
			chunkOverlap: ingestionForm.chunkOverlap,
			segmentSizePolicy: ingestionForm.segmentSizePolicy,
			segmentOrder: ingestionForm.segmentOrder,
			separators: effectiveSeparators.value,
			pagePriority: ingestionForm.pagePriority,
			anchorHeadingPath: ingestionForm.anchorHeadingPath,
			anchorClauseId: ingestionForm.anchorClauseId,
			anchorRowNumber: ingestionForm.anchorRowNumber,
			anchorSpeaker: ingestionForm.anchorSpeaker,
			anchorSentenceIndex: ingestionForm.anchorSentenceIndex,
		};
		const data = await api.triggerIngestion(ingestionForm.spaceId, payload);
		ingestionResult.value = data;
		knowledgeStore.lastIngestionJob = data as any;
		upsertTask({
			spaceId: ingestionForm.spaceId,
			jobId: data.jobId,
			status: data.status,
			sourceLabel: summarizeSource(resolvedSource),
			updatedAt: new Date().toISOString(),
		});

		const remediation = buildIngestionRemediation(data as any);
		if (remediation) {
				ingestionRemediation.value = remediation;
				if (remediation.level === "error") {
					ingestionError.value = remediation.description;
					ingestionStep.value = 4;
					return;
				}
			toast.add({
				color: "warning",
				title: remediation.title,
				description: remediation.description,
			});
		}

		if (ingestionResult.value) {
			// 导入首批样本文档后触发一次 Corpus Check（用于推荐场景/策略包），但不阻塞 UI。
			void startCorpusCheckBestEffort(ingestionForm.spaceId);
			ingestionForm.sourceUri = "";
			ingestionForm.maskingProfile = "";
			selectedFile.value = null;
			ingestionSourceMethod.value = "upload";
			ingestionHistory.value.unshift({
				jobId: data.jobId,
				status: data.status,
				completedAt: new Date().toISOString(),
			});
			if (ingestionHistory.value.length > 5) {
				ingestionHistory.value.pop();
			}
			toast.add({
				color: "success",
				title: t("knowledgeSpaces.ingestion.toast.successTitle", "入库已提交"),
				description: t(
					"knowledgeSpaces.ingestion.toast.successDesc",
					"入库作业已异步触发，将进入任务详情页查看进度。",
				),
			});
			closeIngestionModal();
			await navigateTo(
				`/knowledge-spaces/${encodeURIComponent(ingestionForm.spaceId)}/ingestions/${encodeURIComponent(data.jobId)}`,
			);
		}
	} catch (error) {
		const message = error instanceof Error ? error.message : t("knowledgeSpaces.ingestion.errors.runFailed");
		ingestionError.value = message;
	} finally {
		ingestionSubmitting.value = false;
	}
};

const applyRemediationAction = (action: any) => {
	if (!action) return;
	if (action.kind === "link") {
		navigateTo(String(action.to || "/"));
		return;
	}
	if (action.kind === "event") {
		if (action.event === "enable_ocr") ingestionForm.ocrRequired = true;
		if (action.event === "disable_ocr") ingestionForm.ocrRequired = false;
		ingestionAdvancedOpen.value = true;
	}
};
</script>

<template>
  <section class="px-6 py-8 space-y-8 lg:px-10">
    <header class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm text-gray-500">{{ t("knowledgeSpaces.hero.badge") }}</p>
        <h1 class="text-2xl font-semibold text-gray-900">{{ t("knowledgeSpaces.hero.title") }}</h1>
        <p class="text-gray-600 mt-2">
          {{ t("knowledgeSpaces.hero.description") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-3">
        <component
          :is="action.onClick ? 'button' : 'NuxtLink'"
          v-for="action in quickActions"
          :key="action.title"
          :to="action.onClick ? undefined : action.to"
          :type="action.onClick ? 'button' : undefined"
          class="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition hover:bg-gray-50"
          :class="action.primary ? 'bg-primary-600 text-white border-primary-600 hover:bg-primary-500' : 'border-gray-200 text-gray-700'"
          @click="action.onClick ? action.onClick() : undefined"
        >
          <UIcon :name="action.icon" class="w-5 h-5" />
          <span>{{ action.title }}</span>
        </component>
      </div>
    </header>

		    <UCard v-if="!spacesLoaded || spacesLoading">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">加载中</h2>
            <p class="text-sm text-[var(--text-secondary)]">正在获取当前租户的知识空间列表…</p>
          </div>
          <UButton color="neutral" variant="subtle" :loading="true"> </UButton>
        </div>
      </template>
      <div class="text-sm text-[var(--text-secondary)]">
        <span v-if="spacesError" class="text-red-500">{{ spacesError }}</span>
        <span v-else>如果一直卡住，请确认后端已启动且已登录。</span>
      </div>
	    </UCard>

	    <UCard v-else-if="!hasSpaces">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">快速开始</h2>
            <p class="text-sm text-[var(--text-secondary)]">建议按这个顺序操作：先建空间，再入库，最后用 Playground 验证</p>
          </div>
          <UButton color="primary" icon="i-heroicons-plus-circle" @click="goCreateSpace">新建空间</UButton>
        </div>
      </template>
      <div class="grid gap-4 md:grid-cols-3">
        <div class="rounded-lg border border-[var(--border-color)] p-4">
          <div class="flex items-center gap-2 font-medium text-[var(--text-primary)]">
            <span class="inline-flex h-6 w-6 items-center justify-center rounded-full bg-primary-500/15 text-primary-500">1</span>
            <span>创建知识空间</span>
          </div>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">完成租户/策略/配额/IAM 的向导后，系统才会生成可入库的空间。</p>
        </div>
        <div class="rounded-lg border border-[var(--border-color)] p-4">
          <div class="flex items-center gap-2 font-medium text-[var(--text-primary)]">
            <span class="inline-flex h-6 w-6 items-center justify-center rounded-full bg-primary-500/15 text-primary-500">2</span>
            <span>入库导入内容</span>
          </div>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">创建成功后回到本页，点击“打开入库”，选择来源并提交作业。</p>
        </div>
        <div class="rounded-lg border border-[var(--border-color)] p-4">
          <div class="flex items-center gap-2 font-medium text-[var(--text-primary)]">
            <span class="inline-flex h-6 w-6 items-center justify-center rounded-full bg-primary-500/15 text-primary-500">3</span>
            <span>Playground 验证</span>
          </div>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">对比不同 RAG Profile 的耗时/候选数/降级原因与 citations。</p>
        </div>
      </div>
	    </UCard>

      <UCard v-else>
	        <template #header>
	          <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
	            <div>
	              <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.spaces.title", "空间列表") }}</h2>
	              <p class="text-sm text-[var(--text-secondary)]">
	                {{ t("knowledgeSpaces.spaces.subtitle", "先在这里选中一个空间，然后再进行入库/Playground 等操作。") }}
	              </p>
	            </div>
	            <div class="flex flex-wrap items-center gap-2">
	              <UInput
	                v-model="spaceQuery"
	                class="w-64"
	                :placeholder="t('knowledgeSpaces.spaces.search', '搜索空间…')"
	                icon="i-heroicons-magnifying-glass"
	              />
                <USelect
                  v-model="departmentFilter"
                  :items="departmentItems"
                  option-attribute="label"
                  value-attribute="value"
                  class="w-40"
                />
                <USelect
                  v-model="statusFilter"
                  :items="statusItems"
                  option-attribute="label"
                  value-attribute="value"
                  class="w-40"
                />
	              <UButton color="primary" icon="i-heroicons-plus-circle" @click="goCreateSpace">
	                {{ t("knowledgeSpaces.spaces.actions.create", "新建空间") }}
	              </UButton>
	            </div>
	          </div>
	        </template>

	        <div v-if="spacesError" class="text-sm text-red-500">{{ spacesError }}</div>
	        <div v-else-if="filteredSpaces.length === 0" class="text-sm text-[var(--text-secondary)]">
	          {{ t("knowledgeSpaces.spaces.empty", "没有匹配的空间") }}
	        </div>

          <div v-else class="space-y-3">
            <div class="flex items-center justify-between text-xs text-[var(--text-secondary)]">
              <span>
                {{
                  t("knowledgeSpaces.spaces.pagination.summary", "显示 {start}-{end} / 共 {total} 个空间", {
                    start: paginationInfo.start,
                    end: paginationInfo.end,
                    total: paginationInfo.total,
                  })
                }}
              </span>
              <div class="flex items-center gap-2">
                <span>{{ t("knowledgeSpaces.spaces.pagination.pageSize", "每页") }}</span>
                <USelect
                  :model-value="pagination.pageSize"
                  :items="pageSizeItems"
                  option-attribute="label"
                  value-attribute="value"
                  class="w-20"
                  @update:model-value="(v) => (pagination.pageSize = Number(v))"
                />
              </div>
            </div>

            <UTable :columns="tableColumns" :data="paginatedSpaces" row-key="spaceId" />

            <div class="flex justify-end gap-2">
              <UButton
                color="neutral"
                variant="soft"
                size="sm"
                :disabled="pagination.page <= 1"
                icon="i-heroicons-chevron-left"
                @click="pagination.page = Math.max(1, pagination.page - 1)"
              >
                {{ t("knowledgeSpaces.spaces.pagination.prev", "上一页") }}
              </UButton>
              <UButton
                color="neutral"
                variant="soft"
                size="sm"
                :disabled="pagination.page >= totalPages"
                icon="i-heroicons-chevron-right"
                @click="pagination.page = Math.min(totalPages, pagination.page + 1)"
              >
                {{ t("knowledgeSpaces.spaces.pagination.next", "下一页") }}
              </UButton>
            </div>
          </div>
	      </UCard>

    <!-- 入库入口下沉到空间列表行操作 -->

    <UModal
      v-model:open="ingestionModalOpen"
      :title="t('knowledgeSpaces.ingestion.modal.title', '入库任务')"
      :description="t('knowledgeSpaces.ingestion.modal.desc', '选择空间与来源，触发一次入库作业')"
      fullscreen
      :ui="{
        content: '!inset-0 !w-screen !h-[100dvh] !max-w-none !max-h-none !rounded-none',
        body: 'p-0',
        footer: 'justify-end',
      }"
      :close="{ onClick: closeIngestionModal }"
      prevent-close
    >
      <template #body>
        <div class="mx-auto w-full max-w-6xl px-6 sm:px-10 lg:px-14 py-6">
        <UAlert
          v-if="!hasSpaces"
          class="mb-4"
          color="warning"
          variant="soft"
          title="暂无可选空间"
          description="请先创建知识空间，然后再回到这里发起入库作业。"
        >
          <template #actions>
            <UButton color="primary" size="sm" icon="i-heroicons-plus-circle" @click="goCreateSpace">新建空间</UButton>
          </template>
        </UAlert>
        <UAlert
          v-if="ingestionError"
          class="mb-4"
          :color="ingestionRemediation?.level === 'warning' ? 'warning' : 'error'"
          variant="soft"
          :title="ingestionRemediation?.title || t('knowledgeSpaces.ingestion.toast.errorTitle', '入库失败')"
          :description="ingestionError"
        >
          <template v-if="ingestionRemediation?.actions?.length" #actions>
            <UButton
              v-for="action in ingestionRemediation.actions"
              :key="action.label"
              size="sm"
              :color="action.kind === 'link' ? 'primary' : 'neutral'"
              :variant="action.kind === 'link' ? 'solid' : 'soft'"
              @click="applyRemediationAction(action)"
            >
              {{ action.label }}
            </UButton>
          </template>
        </UAlert>
        <UAlert
          v-if="embeddingLimitHint"
          class="mb-4"
          color="warning"
          variant="soft"
          title="Embedding 上限提示"
          :description="embeddingLimitHint"
        />
        <div class="mb-4 flex items-center justify-between gap-2 text-sm">
          <div class="text-[var(--text-secondary)]">
            {{ t("knowledgeSpaces.ingestion.wizard.step", { n: ingestionStep, total: 4 }) }}
          </div>
          <UBadge color="neutral" variant="soft">
            {{ t("knowledgeSpaces.ingestion.wizard.stepBadge", { n: ingestionStep }) }}
          </UBadge>
        </div>

        <UForm id="knowledge-ingestion-form" :state="ingestionForm" class="space-y-4" @submit="submitIngestion">
          <template v-if="ingestionStep === 1">
            <UFormField :label="t('knowledgeSpaces.ingestion.spaceId')" required>
              <USelectMenu
                v-model="ingestionForm.spaceId"
                :items="ingestionSpaceItems"
                value-key="value"
                label-key="label"
                class="w-full"
                :placeholder="t('knowledgeSpaces.ingestion.selectSpace')"
              />
            </UFormField>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <UFormField :label="t('knowledgeSpaces.ingestion.sourceType')">
                <USelect
                  v-model="ingestionForm.format"
                  :items="sourceOptions"
                  class="w-full"
                />
              </UFormField>

              <UFormField :label="t('knowledgeSpaces.ingestion.sourceMethod.label')">
                <USelect v-model="ingestionSourceMethod" :items="sourceMethodOptions" class="w-full" />
                <template #help>
                  <span v-if="ingestionSourceMethod === 'url'" class="text-xs text-[var(--text-secondary)]">
                    {{ t("knowledgeSpaces.ingestion.hints.urlPublic") }}
                  </span>
                </template>
              </UFormField>
            </div>

            <UFormField v-if="ingestionSourceMethod === 'upload'" :label="t('knowledgeSpaces.ingestion.uploadFile')" required>
              <input
                type="file"
                accept=".pdf,.md,.markdown,.txt,.xlsx,.doc,.docx,.csv,.html,.htm,.sql"
                @change="handleFileChange"
                class="block w-full text-sm"
              />
              <template #help>
                <div class="space-y-2">
                  <div class="text-xs text-[var(--text-secondary)]">
                    {{ t("knowledgeSpaces.ingestion.uploadHint") }}
                  </div>
                  <div class="flex items-start gap-2">
                    <UCheckbox v-model="ingestionRetainSource" />
                    <div class="text-xs text-[var(--text-secondary)] leading-5">
                      {{ t("knowledgeSpaces.ingestion.retainSource", "上传后保留到媒体库（推荐）") }}
                      <div class="mt-1 text-[var(--text-tertiary)]">
                        {{
                          t(
                            "knowledgeSpaces.ingestion.retainSourceHint",
                            "关闭后仍会临时上传到对象存储以便入库抓取；后续将支持按策略自动清理。",
                          )
                        }}
                      </div>
                    </div>
                  </div>
                </div>
              </template>
            </UFormField>

            <UFormField v-else :label="t('knowledgeSpaces.ingestion.sourceUri')" required>
              <UInput v-model="ingestionForm.sourceUri" class="w-full" placeholder="https://..." icon="i-heroicons-link" />
              <template #help>
                <span class="text-xs text-[var(--text-secondary)]">
                  {{ t("knowledgeSpaces.ingestion.hints.urlPublic") }}
                </span>
              </template>
            </UFormField>
          </template>

          <template v-else-if="ingestionStep === 2">
            <UAlert
              v-if="ocrRecommendation"
              color="warning"
              variant="soft"
              class="mb-4"
              :title="ocrRecommendation.title || '扫描件占比偏高：建议启用 OCR'"
              :description="ocrRecommendation.risk || '若不启用 OCR，检索召回与引用覆盖会显著下降。'"
            >
              <template #actions>
                <UButton color="primary" size="sm" icon="i-heroicons-shopping-bag" @click="openOcrPluginMarket">
                  去安装 OCR 插件
                </UButton>
                <UButton color="neutral" variant="soft" size="sm" @click="applyOcrSuggestion">
                  本次入库启用 OCR
                </UButton>
              </template>
            </UAlert>
            <div class="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_420px]">
              <div class="space-y-4">
                <UFormField :label="t('knowledgeSpaces.ingestion.strategyPackage', '策略包（A0–O）')" required>
                  <USelect v-model="ingestionStrategyPackageKey" :items="strategyPackageItems" class="w-full" />
                  <template #help>
                    <div class="text-xs text-[var(--text-secondary)]">
                      {{ selectedIngestionPackage?.summary || "选择一个策略包，系统会自动映射场景与 Profile 预设。" }}
                    </div>
                    <div v-if="ingestionStrategyAutoHint" class="mt-1 text-xs text-[var(--text-secondary)]">
                      {{ ingestionStrategyAutoHint }}
                    </div>
                    <div v-if="canApplyAutoSuggestion" class="mt-2">
                      <UButton
                        size="xs"
                        color="neutral"
                        variant="soft"
                        icon="i-heroicons-sparkles"
                        type="button"
                        @click="applyAutoSuggestionNow"
                      >
                        {{ t("knowledgeSpaces.ingestion.sceneSuggestion.apply", { label: autoSuggestedStrategyLabel }) }}
                      </UButton>
                    </div>
                  </template>
                </UFormField>

                <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <UFormField :label="t('knowledgeSpaces.ingestion.priority')">
                    <USelect
                      v-model="ingestionForm.priority"
                      :items="priorityOptions"
                      class="w-full"
                    />
                  </UFormField>
                  <div class="flex items-end">
                    <UButton
                      color="neutral"
                      variant="subtle"
                      type="button"
                      class="w-full justify-center"
                      @click="ingestionAdvancedOpen = !ingestionAdvancedOpen"
                    >
                      {{ ingestionAdvancedOpen ? t("knowledgeSpaces.ingestion.advanced.hide") : t("knowledgeSpaces.ingestion.advanced.show") }}
                    </UButton>
                  </div>
                </div>

                <div v-if="ingestionAdvancedOpen" class="rounded-lg border border-gray-200 p-4 space-y-4">
                  <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                    <UFormField :label="t('knowledgeSpaces.ingestion.maskingProfile')">
                      <UInput v-model="ingestionForm.maskingProfile" class="w-full" placeholder="masking.profile.v1" />
                    </UFormField>
                    <UFormField label="Processor Profile">
                      <UInput v-model="ingestionForm.processorProfile" class="w-full" placeholder="builtin/default" />
                    </UFormField>
                    <UFormField label="Ingestion Profile">
                      <UInput v-model="ingestionForm.ingestionProfile" class="w-full" placeholder="default" />
                    </UFormField>
                  </div>

                  <UCheckbox v-model="ingestionForm.ocrRequired">OCR required (blocked if unavailable)</UCheckbox>
                </div>
              </div>

              <div class="rounded-lg border border-gray-200 p-4 space-y-3 text-sm max-h-[calc(100dvh-12rem)] overflow-auto">
                <div class="flex items-start justify-between gap-2">
                  <div>
                    <div class="font-medium">{{ t("knowledgeSpaces.ingestion.guidance.title", "引导说明") }}</div>
                    <div class="text-[var(--text-secondary)]">
                      {{
                        t(
                          "knowledgeSpaces.ingestion.guidance.desc",
                          "策略包是主线选择；场景仅用于说明与默认配置映射。",
                        )
                      }}
                    </div>
                  </div>
                  <UBadge color="neutral" variant="soft">Strategy</UBadge>
                </div>

                <div class="text-[var(--text-secondary)]">
                  {{ t("knowledgeSpaces.ingestion.guidance.currentSelection", "当前选择") }}：
                  {{ selectedIngestionPackage?.label || ingestionStrategyPackageKey }}
                </div>

                <div class="space-y-1">
                  <div class="text-xs text-[var(--text-secondary)]">
                    推荐 Profile
                  </div>
                  <div class="flex flex-wrap gap-2">
                    <UBadge color="primary" variant="soft">
                      {{ profileLabel(derivedIngestionProfileKey) }}
                    </UBadge>
                    <UBadge color="neutral" variant="soft">
                      {{ strategyPackageCouplingLabel }}
                    </UBadge>
                  </div>
                </div>

                <div class="space-y-1">
                  <div class="text-xs text-[var(--text-secondary)]">
                    适用场景（映射说明）
                  </div>
                  <div class="flex flex-wrap gap-2">
                    <UBadge v-for="scene in strategyPackageSceneLabels" :key="scene.key" color="neutral" variant="soft">
                      {{ scene.label }}
                    </UBadge>
                    <span v-if="!strategyPackageSceneLabels.length" class="text-[var(--text-tertiary)]">-</span>
                  </div>
                  <div class="text-[var(--text-secondary)] mt-2">
                    {{ strategyPackageDecisionHint }}
                  </div>
                </div>

                <div class="text-xs text-[var(--text-secondary)] pt-1">
                  下一步会基于策略包自动调整分割/锚点默认值（你仍可手动覆盖）。
                </div>

                <div class="flex flex-wrap gap-2 pt-1">
                  <UButton
                    as="a"
                    :href="overviewGuideHref"
                    target="_blank"
                    size="xs"
                    color="neutral"
                    variant="soft"
                    icon="i-heroicons-map"
                  >
                    {{ t("knowledgeSpaces.ingestion.guidance.openOverview", "查看总览与映射") }}
                  </UButton>
                  <UButton
                    as="a"
                    :href="sceneGuideHref"
                    target="_blank"
                    size="xs"
                    color="neutral"
                    variant="soft"
                    icon="i-heroicons-book-open"
                  >
                    {{ t("knowledgeSpaces.ingestion.guidance.openScene", "查看场景指引") }}
                  </UButton>
                </div>
              </div>
            </div>
          </template>

          <template v-else-if="ingestionStep === 3">
            <div class="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_420px]">
              <div class="space-y-4">
                <UAlert
                  color="neutral"
                  variant="soft"
                  title="分割策略（Segment）"
                  description="根据“策略包 + 文档格式”自动给出默认值；你可以在这里覆盖。"
                />

                <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div class="md:col-span-2">
                    <div class="rounded-lg border border-gray-200 p-3">
                      <div class="text-sm font-medium">切分优先级（可拖拽排序）</div>
                      <div class="text-xs text-[var(--text-secondary)] mt-1">
                        从上到下依次执行。未勾选分页时，“分页”步骤自动跳过。
                      </div>
                      <div class="mt-3 space-y-2">
                        <div
                          v-for="key in ingestionForm.segmentOrder"
                          :key="key"
                          class="flex items-center justify-between rounded-md border border-gray-200 bg-white px-3 py-2 text-sm"
                          draggable="true"
                          @dragstart="onSegmentDragStart(key)"
                          @dragover="onSegmentDragOver"
                          @drop="onSegmentDrop(key)"
                        >
                          <div class="flex items-center gap-2 min-w-0">
                            <span class="i-heroicons-bars-3 text-gray-400" />
                            <div class="min-w-0">
                              <div class="truncate">{{ segmentOrderLabels[key] }}</div>
                              <div class="text-xs text-[var(--text-tertiary)] truncate">
                                {{ segmentOrderHelp[key] }}
                                <span v-if="key === 'page' && !ingestionForm.pagePriority">（未启用）</span>
                              </div>
                            </div>
                          </div>
                          <div class="flex items-center gap-2">
                            <UCheckbox
                              v-if="key === 'page'"
                              v-model="ingestionForm.pagePriority"
                            />
                            <UCheckbox
                              v-if="key === 'segment'"
                              :model-value="isSegmentStepEnabled('segment')"
                              @update:model-value="(v:boolean) => toggleSegmentStep('segment', v)"
                            />
                            <UCheckbox
                              v-if="key === 'separator'"
                              :model-value="isSegmentStepEnabled('separator')"
                              @update:model-value="(v:boolean) => toggleSegmentStep('separator', v)"
                            />
                            <UButton
                              size="xs"
                              color="neutral"
                              variant="ghost"
                              icon="i-heroicons-chevron-up"
                              :disabled="ingestionForm.segmentOrder.indexOf(key) === 0"
                              @click="moveSegmentOrder(ingestionForm.segmentOrder.indexOf(key), ingestionForm.segmentOrder.indexOf(key) - 1)"
                            />
                            <UButton
                              size="xs"
                              color="neutral"
                              variant="ghost"
                              icon="i-heroicons-chevron-down"
                              :disabled="ingestionForm.segmentOrder.indexOf(key) === ingestionForm.segmentOrder.length - 1"
                              @click="moveSegmentOrder(ingestionForm.segmentOrder.indexOf(key), ingestionForm.segmentOrder.indexOf(key) + 1)"
                            />
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  <UFormField label="分段模式（分隔策略）" required>
                    <USelect
                      v-model="ingestionForm.segmentMode"
                      :items="segmentModeOptions"
                      class="w-full"
                    />
                    <template #help>
                      <span class="text-xs text-[var(--text-secondary)]">
                        {{ segmentModeHint }}
                      </span>
                    </template>
                  </UFormField>
                  <UFormField v-if="showPagePriority" label="分页优先（仅 PDF）">
                    <UCheckbox v-model="ingestionForm.pagePriority">
                      先按页切分，再在页内按分段模式处理
                    </UCheckbox>
                    <template #help>
                      <span class="text-xs text-[var(--text-secondary)]">
                        勾选后不会跨页合并，页内仍按分隔符/长度窗口切分。
                      </span>
                    </template>
                  </UFormField>
                  <UFormField :label="t('knowledgeSpaces.ingestion.chunkSize', '分段长度（字符/近似 token）')">
                    <UInput
                      v-model.number="ingestionForm.chunkSize"
                      type="number"
                      min="0"
                      max="20000"
                      step="50"
                      class="w-full"
                      placeholder="800"
                    />
                    <template #help>
                      <span class="text-xs text-[var(--text-secondary)]">
                        {{ t("knowledgeSpaces.ingestion.chunkSizeHint", "0 表示只按“分段模式”拆分；>0 会按字数策略处理（目标=先合并短段，上限=只切超长）。") }}
                      </span>
                      <div class="mt-1 text-xs text-[var(--text-secondary)]">
                        {{ chunkSizeSourceHint }}
                      </div>
                      <div v-if="embeddingLimitHint" class="mt-1 text-xs text-[var(--text-warning)]">
                        {{ embeddingLimitHint }}
                      </div>
                    </template>
                  </UFormField>
                  <UFormField label="字数策略（chunkSize 生效方式）">
                    <USelect
                      v-model="ingestionForm.segmentSizePolicy"
                      :items="[
                        { label: '目标长度（会合并短段）', value: 'target' },
                        { label: '长度上限（只切超长）', value: 'cap' },
                      ]"
                      class="w-full"
                    />
                    <template #help>
                      <span class="text-xs text-[var(--text-secondary)]">
                        {{ chunkSizePolicyHelp }}
                      </span>
                    </template>
                  </UFormField>
                  <UFormField :label="t('knowledgeSpaces.ingestion.chunkOverlap', '分段重叠（字符）')">
                    <UInput
                      v-model.number="ingestionForm.chunkOverlap"
                      type="number"
                      min="0"
                      max="5000"
                      step="20"
                      class="w-full"
                      placeholder="120"
                    />
                    <template #help>
                      <span class="text-xs text-[var(--text-secondary)]">
                        {{ t("knowledgeSpaces.ingestion.chunkOverlapHint", "用于减少跨段断裂；过大将增加成本。") }}
                      </span>
                    </template>
                  </UFormField>
                </div>

                <div class="rounded-lg border border-gray-200 p-4 space-y-3 text-sm">
                  <div class="font-medium">锚点与分隔符（Anchors）</div>
                  <div class="text-[var(--text-secondary)]">
                    锚点会写入 chunk metadata，用于引用定位、层次索引（J Hier）与 KG provenance。不同场景默认值不同，可在此覆盖。
                  </div>
                  <div class="text-xs text-[var(--text-secondary)]">
                    {{ segmentAnchorHint }}
                  </div>
                  <div class="space-y-2">
                    <div class="font-medium text-sm">
                      {{ t("knowledgeSpaces.ingestion.separators", "分隔符（优先边界，可选）") }}
                    </div>
                    <div class="text-xs text-[var(--text-secondary)]">
                      {{ t("knowledgeSpaces.ingestion.separatorsHint", "用于在窗口切分前，优先在自然边界断开（如空行/句号/分号）。对“表格行”通常不需要。") }}
                    </div>
                    <div v-if="ingestionForm.segmentMode === 'table_row'" class="text-xs text-[var(--text-secondary)]">
                      当前为“表格行/记录”模式：建议保持分隔符为空，避免破坏行级索引语义。
                    </div>

                    <USelect
                      v-model="separatorSelected"
                      :items="separatorOptions"
                      option-attribute="label"
                      value-attribute="value"
                      class="w-full"
                    />

                    <UFormField
                      v-if="separatorSelected === SEPARATOR_CUSTOM_VALUE"
                      :label="t('knowledgeSpaces.ingestion.separatorsCustom', '自定义分隔符')"
                    >
                      <UInput v-model="separatorCustomText" placeholder="例如：\\n\\n 或 。 或 }" />
                      <template #help>
                        <span class="text-xs text-[var(--text-secondary)]">
                          {{ t("knowledgeSpaces.ingestion.separatorsCustomHint", "填写一个分隔符；如需换行分隔符请填写 \\\\n 或 \\\\n\\\\n。") }}
                        </span>
                      </template>
                    </UFormField>

                    <div class="text-xs text-[var(--text-secondary)]">
                      最终生效：{{ effectiveSeparatorsPreview }}
                    </div>
                  </div>
                  <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                    <div class="flex items-start gap-2">
                      <UCheckbox v-model="ingestionForm.anchorHeadingPath" />
                      <div class="min-w-0">
                        <div class="text-sm">标题路径（heading_path）</div>
                        <div class="text-xs text-[var(--text-secondary)]">适合 SOP/长文：保留章节锚点与层级检索。</div>
                      </div>
                    </div>
                    <div class="flex items-start gap-2">
                      <UCheckbox v-model="ingestionForm.anchorClauseId" />
                      <div class="min-w-0">
                        <div class="text-sm">条款编号（clause_id）</div>
                        <div class="text-xs text-[var(--text-secondary)]">适合合同/规则：便于精确引用与合规核对。</div>
                      </div>
                    </div>
                    <div class="flex items-start gap-2">
                      <UCheckbox v-model="ingestionForm.anchorRowNumber" />
                      <div class="min-w-0">
                        <div class="text-sm">表格行号（row_number）</div>
                        <div class="text-xs text-[var(--text-secondary)]">适合台账/清单：行级命中更可解释。</div>
                      </div>
                    </div>
                    <div class="flex items-start gap-2">
                      <UCheckbox v-model="ingestionForm.anchorSpeaker" />
                      <div class="min-w-0">
                        <div class="text-sm">发言人（speaker）</div>
                        <div class="text-xs text-[var(--text-secondary)]">适合工单/聊天：按轮次+角色定位。</div>
                      </div>
                    </div>
                    <div class="flex items-start gap-2">
                      <UCheckbox v-model="ingestionForm.anchorSentenceIndex" />
                      <div class="min-w-0">
                        <div class="text-sm">句子序号（sentence_idx）</div>
                        <div class="text-xs text-[var(--text-secondary)]">适合语义切块：提升引用与邻接扩展。</div>
                      </div>
                    </div>
                  </div>
                </div>


              </div>

              <div class="rounded-lg border border-gray-200 p-4 space-y-3 text-sm max-h-[calc(100dvh-12rem)] overflow-auto">
                <div class="flex items-start justify-between gap-2">
                  <div>
                    <div class="font-medium">为什么要单独配置这一页？</div>
                    <div class="text-[var(--text-secondary)]">
                      不同场景对“边界/锚点/粒度”敏感度不同：合同偏条款，论文偏语义与章节，表格偏行记录，SQL 偏对象/块。
                    </div>
                  </div>
                  <UBadge color="neutral" variant="soft">Segment</UBadge>
                </div>

                <UAlert
                  color="warning"
                  variant="soft"
                  title="提示"
                  description="我们会把“标题路径/条款号/行号/发言人/句子序号”等结构锚点写进每个 chunk 的 metadata，方便检索命中后快速回到原文位置。如果你希望做到更精确的定位（例如 PDF 第几页、OCR 框选坐标/bbox 叠框），需要选用支持 PDF/OCR 的 processor/profile。"
                />

                <div class="rounded-lg border border-gray-200 p-4 space-y-2 text-sm">
                  <div class="font-medium">将使用的分割策略（可覆盖）</div>
                  <div class="text-[var(--text-secondary)]">
                    顺序：{{ segmentPreviewOrder.join(" → ") }}
                  </div>
                  <div class="text-[var(--text-secondary)]">分页优先：{{ ingestionForm.pagePriority ? "是" : "否" }}</div>
                  <div class="text-[var(--text-secondary)]">模式：{{ segmentPreviewMode }}</div>
                  <div class="text-[var(--text-secondary)]">
                    Chunk：{{ ingestionForm.chunkSize }} / Overlap：{{ ingestionForm.chunkOverlap }}（{{ chunkSizePolicyLabel }}）
                  </div>
                  <div class="text-[var(--text-secondary)]">分隔符：{{ segmentPreviewSeparators }}</div>
                  <div class="text-[var(--text-secondary)]">
                    Anchors：
                    <span v-if="ingestionForm.anchorHeadingPath">heading_path</span>
                    <span v-if="ingestionForm.anchorClauseId" class="ml-2">clause_id</span>
                    <span v-if="ingestionForm.anchorRowNumber" class="ml-2">row_number</span>
                    <span v-if="ingestionForm.anchorSpeaker" class="ml-2">speaker</span>
                    <span v-if="ingestionForm.anchorSentenceIndex" class="ml-2">sentence_idx</span>
                    <span
                      v-if="!ingestionForm.anchorHeadingPath && !ingestionForm.anchorClauseId && !ingestionForm.anchorRowNumber && !ingestionForm.anchorSpeaker && !ingestionForm.anchorSentenceIndex"
                    >
                      -
                    </span>
                  </div>
                </div>

                <div class="rounded-md border border-[var(--border-color)] bg-[var(--card-bg)] p-3 text-xs">
                  <div class="font-medium text-[var(--text-primary)]">联动逻辑</div>
                  <div class="mt-1 text-[var(--text-secondary)] whitespace-pre-line">
                    {{ chunkingFlowHint }}
                  </div>
                </div>

                <div class="flex flex-wrap gap-2 pt-1">
                  <UButton
                    as="a"
                    :href="sceneGuideHref"
                    target="_blank"
                    size="xs"
                    color="neutral"
                    variant="soft"
                    icon="i-heroicons-book-open"
                  >
                    查看该场景分割建议
                  </UButton>
                </div>
              </div>
            </div>
          </template>

          <template v-else>
            <div class="rounded-lg border border-gray-200 p-4 space-y-2 text-sm">
              <div class="font-medium">{{ t("knowledgeSpaces.ingestion.confirm.title") }}</div>
              <div class="text-[var(--text-secondary)]">
                {{ t("knowledgeSpaces.ingestion.confirm.space") }}：{{ ingestionSpaceItems.find(i => i.value === ingestionForm.spaceId)?.label || ingestionForm.spaceId.slice(0, 8) }}
              </div>
              <div class="text-[var(--text-secondary)]">
                {{ t("knowledgeSpaces.ingestion.confirm.source") }}：{{ ingestionForm.sourceUri || (selectedFile ? selectedFile.name : "-") }}
              </div>
              <div class="text-[var(--text-secondary)]">
                {{ t("knowledgeSpaces.ingestion.confirm.template") }}：{{ selectedIngestionPackage?.label }}（{{ profileLabel(derivedIngestionProfileKey) }}）
              </div>
              <div class="text-[var(--text-secondary)]">
                策略包：{{ ingestionStrategyPackageKey }}
              </div>
              <div class="text-[var(--text-secondary)]">
                分页优先：{{ ingestionForm.pagePriority ? "是" : "否" }}
              </div>
              <div class="text-[var(--text-secondary)]">
                分割：{{ ingestionForm.segmentMode }} · {{ ingestionForm.chunkSize }}/{{ ingestionForm.chunkOverlap }}（{{ chunkSizePolicyLabel }}）
              </div>
              <div class="text-[var(--text-secondary)]">
                Anchors：
                <span v-if="ingestionForm.anchorHeadingPath">heading_path</span>
                <span v-if="ingestionForm.anchorClauseId" class="ml-2">clause_id</span>
                <span v-if="ingestionForm.anchorRowNumber" class="ml-2">row_number</span>
                <span v-if="ingestionForm.anchorSpeaker" class="ml-2">speaker</span>
                <span v-if="ingestionForm.anchorSentenceIndex" class="ml-2">sentence_idx</span>
                <span
                  v-if="!ingestionForm.anchorHeadingPath && !ingestionForm.anchorClauseId && !ingestionForm.anchorRowNumber && !ingestionForm.anchorSpeaker && !ingestionForm.anchorSentenceIndex"
                >
                  -
                </span>
              </div>
            </div>
          </template>
        </UForm>
        </div>
      </template>

      <template #footer>
        <div class="flex items-center justify-between gap-2 w-full">
          <div class="flex gap-2">
            <UButton color="neutral" variant="subtle" type="button" :disabled="ingestionStep === 1" @click="goPrevStep">
              {{ t("knowledgeSpaces.ingestion.wizard.prev") }}
            </UButton>
            <UButton
              v-if="ingestionStep < 4"
              color="primary"
              variant="soft"
              type="button"
              :disabled="!canGoNext"
              @click="goNextStep"
            >
              {{ t("knowledgeSpaces.ingestion.wizard.next") }}
            </UButton>
          </div>
          <div class="flex gap-2">
            <UButton color="neutral" variant="subtle" type="button" @click="closeIngestionModal">
              {{ t("common.cancel", "取消") }}
            </UButton>
            <UButton
              color="primary"
              type="submit"
              form="knowledge-ingestion-form"
              :loading="ingestionSubmitting"
              :disabled="ingestionStep !== 4 || !canSubmit"
            >
              {{ t("knowledgeSpaces.ingestion.actions.ingestNow") }}
            </UButton>
          </div>
        </div>
      </template>
    </UModal>

    <div class="flex justify-end">
      <UButton color="neutral" variant="subtle" type="button" @click="systemPanelsOpen = !systemPanelsOpen">
        {{ systemPanelsOpen ? t("knowledgeSpaces.systemPanels.hide", "收起系统面板") : t("knowledgeSpaces.systemPanels.show", "显示系统面板") }}
      </UButton>
    </div>

    <section v-if="systemPanelsOpen" class="space-y-6">
      <QaBridgeStatusCard :status="qaStatus" @refresh="refreshQaStatus" />

      <UCard>
        <template #header>
          <div class="flex items-center justify-between">
            <div>
              <h2 class="text-lg font-semibold">{{ t("knowledgeSpaces.timeline.title") }}</h2>
              <p class="text-gray-500 text-sm">{{ t("knowledgeSpaces.timeline.description") }}</p>
            </div>
            <UBadge color="orange" variant="soft">{{ t("knowledgeSpaces.timeline.badge") }}</UBadge>
          </div>
        </template>

        <div class="grid gap-6 md:grid-cols-3">
          <article
            v-for="item in timelinePlaceholders"
            :key="item.title"
            class="rounded-xl border border-dashed border-gray-200 p-4 bg-gray-50"
          >
            <h3 class="font-medium text-gray-900">{{ item.title }}</h3>
            <p class="text-sm text-gray-600 mt-2">{{ item.description }}</p>
          </article>
        </div>
      </UCard>
    </section>

    <!-- 右下角：入库任务悬浮入口 -->
    <div class="fixed bottom-6 right-6 z-50 flex flex-col items-end gap-2">
      <UButton
        color="primary"
        variant="solid"
        icon="i-heroicons-queue-list"
        class="shadow-lg"
        @click="ingestionTaskPanelOpen = true"
      >
        入库任务
        <UBadge v-if="runningIngestionCount" class="ml-2" color="warning" variant="soft">
          {{ runningIngestionCount }}
        </UBadge>
      </UButton>
    </div>

    <USlideover
      v-model:open="ingestionTaskPanelOpen"
      title="入库任务"
      description="查看入库任务的状态与结果"
      :ui="{ content: 'w-[calc(100vw-2rem)] max-w-md', body: 'p-4 space-y-3' }"
    >
      <template #body>
        <div class="flex items-center justify-between">
          <div class="text-sm text-[var(--text-secondary)]">
            {{ hasRunningIngestionTasks ? "正在入库中…" : "暂无进行中的入库任务" }}
          </div>
          <UButton color="neutral" variant="soft" size="xs" icon="i-heroicons-arrow-path" @click="refreshIngestionTasks">
            刷新
          </UButton>
        </div>

        <UAlert
          v-if="!ingestionTasks.length"
          color="neutral"
          variant="soft"
          title="暂无任务"
          description="你触发入库后，会在这里看到任务进度与结果。"
        />

        <div v-else class="space-y-2">
	          <div
	            v-for="t in ingestionTasks"
	            :key="t.jobId"
	            class="rounded-lg border border-gray-200 bg-white p-3 space-y-1"
	          >
	            <div class="flex items-start justify-between gap-2">
	              <div class="min-w-0">
	                <div class="text-sm font-medium truncate">{{ taskSpaceLabel(t.spaceId) }}</div>
	                <div class="text-xs text-[var(--text-secondary)] truncate">来源：{{ t.sourceLabel }}</div>
	              </div>
	              <div class="flex items-center gap-2">
	                <UBadge
	                  :color="t.status === 'completed' ? 'success' : t.status === 'failed' ? 'error' : t.status === 'blocked' ? 'warning' : 'neutral'"
	                  variant="soft"
	                >
	                  {{ t.status }}
	                </UBadge>
	                <UButton
	                  v-if="t.status === 'completed'"
	                  size="xs"
	                  color="primary"
	                  variant="soft"
	                  @click="navigateTo(`/knowledge-spaces/${encodeURIComponent(t.spaceId)}/ingestions/${encodeURIComponent(t.jobId)}`)"
	                >
	                  预览切块
	                </UButton>
	              </div>
	            </div>
	            <div class="text-xs text-[var(--text-tertiary)]">任务：{{ t.jobId.slice(0, 8) }}… · 更新：{{ new Date(t.updatedAt).toLocaleTimeString() }}</div>
	          </div>
	        </div>
	      </template>
	    </USlideover>
  </section>
</template>
