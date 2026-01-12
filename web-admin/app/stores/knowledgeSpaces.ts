import { defineStore } from "pinia";
import {
  useKnowledgeSpaces,
  type KnowledgeSpacePayload,
  type KnowledgeSpaceRecord,
  type IngestionJobPayload,
  type IngestionJobRecord,
} from "~/composables/useKnowledgeSpaces";
import {
  SCENE_STRATEGY_CATALOG,
  type SceneKey,
  type StrategyBundleKey,
  type IndexPrereqKey,
} from "~/constants/sceneStrategyCatalog";

interface WizardState {
  step: number;
  form: KnowledgeSpacePayload;
  iamEmail: string;
  sceneKey: SceneKey;
  bundleKey: StrategyBundleKey;
  sampleDoc: {
    enabled: boolean;
    format: IngestionJobPayload["format"];
    sourceUri: string;
    ocrRequired: boolean;
  };
  slaSeconds: number;
  slaStartedAt: number | null;
  intervalId: ReturnType<typeof setInterval> | null;
  loading: boolean;
  error: string | null;
  status: "idle" | "success" | "error";
  lastSpace: KnowledgeSpaceRecord | null;
  runCorpusCheckAfterCreate: boolean;
  lastCorpusCheckJob: any | null;
  lastIngestionJob: IngestionJobRecord | null;
}

const DEFAULT_FORM: KnowledgeSpacePayload = {
  spaceName: "",
  departmentCode: "",
  policyTemplateVersionId: "default-v1",
  ingestionProfileKey: "p1_general",
  indexProfileKey: "p1_general",
  ragProfileKey: "p1_general",
  featureFlags: [],
  quotas: {
    cpuCores: 4,
    storageGb: 200,
    ingestionConcurrency: 2,
  },
};

export const useKnowledgeSpaceStore = defineStore("knowledgeSpaceWizard", {
  state: (): WizardState => ({
    step: 1,
    form: { ...DEFAULT_FORM },
    iamEmail: "",
    sceneKey: "sop",
    bundleKey: "p1_general",
    sampleDoc: {
      enabled: false,
      format: "pdf",
      sourceUri: "",
      ocrRequired: false,
    },
    slaSeconds: 120,
    slaStartedAt: null,
    intervalId: null,
    loading: false,
    error: null,
    status: "idle",
    lastSpace: null,
    runCorpusCheckAfterCreate: true,
    lastCorpusCheckJob: null,
    lastIngestionJob: null,
  }),
  getters: {
    slaRemaining(state) {
      if (!state.slaStartedAt) return state.slaSeconds;
      const elapsed = Math.floor((Date.now() - state.slaStartedAt) / 1000);
      return Math.max(state.slaSeconds - elapsed, 0);
    },
    isBasicInfoValid(state): boolean {
      return Boolean(
        state.form.spaceName && state.form.departmentCode,
      );
    },
    isPolicyStepValid(state): boolean {
      return Boolean(state.form.policyTemplateVersionId);
    },
    isQuotaStepValid(state): boolean {
      const { cpuCores, storageGb, ingestionConcurrency } = state.form.quotas;
      return cpuCores > 0 && storageGb >= 50 && ingestionConcurrency > 0;
    },
    wizardCompleted(state): boolean {
      return state.status === "success" && !!state.lastSpace;
    },
  },
  actions: {
    nextStep() {
      if (this.step < 4) {
        this.step += 1;
      }
    },
    prevStep() {
      if (this.step > 1) {
        this.step -= 1;
      }
    },
    reset() {
      this.$reset();
      this.form = { ...DEFAULT_FORM };
    },
    setSceneAndBundle(sceneKey: SceneKey, bundleKey?: StrategyBundleKey) {
      const scene = SCENE_STRATEGY_CATALOG.scenes[sceneKey];
      if (!scene) return;

      const nextBundle = bundleKey && scene.allowedBundles.includes(bundleKey)
        ? bundleKey
        : scene.defaultBundle;

      this.sceneKey = sceneKey;
      this.bundleKey = nextBundle;

      // 绑定/切换三类 Profile（与策略包同 key：p0/p1/p2/p3）。
      this.form.ingestionProfileKey = nextBundle;
      this.form.indexProfileKey = nextBundle;
      this.form.ragProfileKey = nextBundle;

      // 记录为 feature flags（后端可用来做校验/推荐/审计）。
      // 保持单值：先清理旧的 scene/bundle 标记。
      this.form.featureFlags = (this.form.featureFlags || []).filter(
        (f) => !f.startsWith("rag.scene:") && !f.startsWith("rag.bundle:"),
      );
      this.setFeatureFlag("rag.scene:" + sceneKey, true);
      this.setFeatureFlag("rag.bundle:" + nextBundle, true);

      // 兼容旧字段：仍保留 rag.guided 标志，但不再作为主入口。
      this.setFeatureFlag("rag.guided", sceneKey === "custom_expert");
    },
    setFeatureFlag(flag: string, enabled: boolean) {
      const normalized = flag.trim().toLowerCase();
      const index = this.form.featureFlags.findIndex(
        (item) => item === normalized,
      );
      if (enabled && index === -1) {
        this.form.featureFlags.push(normalized);
      }
      if (!enabled && index !== -1) {
        this.form.featureFlags.splice(index, 1);
      }
    },
    computeEnabledIndexChannels(): Array<"dense" | "sparse" | "hier" | "kg" | "time" | "structured"> {
      const scene = SCENE_STRATEGY_CATALOG.scenes[this.sceneKey];
      const bundle = SCENE_STRATEGY_CATALOG.bundles[this.bundleKey];
      const idx = new Set<string>();
      for (const k of scene?.prerequisites.index ?? []) idx.add(k);
      for (const k of bundle?.prerequisites ?? []) idx.add(k);

      const map = (key: IndexPrereqKey): Array<"dense" | "sparse" | "hier" | "kg" | "time" | "structured"> => {
        switch (key) {
          case "index.dense":
            return ["dense"];
          case "index.sparse":
            return ["sparse"];
          case "index.hier":
            return ["hier"];
          case "index.kg":
            return ["kg"];
          case "index.time_fields":
            return ["time"];
          case "index.structured_fields":
            return ["structured"];
        }
      };

      const out: Array<"dense" | "sparse" | "hier" | "kg" | "time" | "structured"> = [];
      for (const k of idx) {
        if (k.startsWith("index.")) {
          out.push(...map(k as IndexPrereqKey));
        }
      }
      const order = ["dense", "sparse", "hier", "kg", "time", "structured"] as const;
      return order.filter((x) => out.includes(x));
    },
    setQuota(key: keyof KnowledgeSpacePayload["quotas"], value: number) {
      this.form.quotas[key] = value;
    },
    startSLAClock() {
      this.slaStartedAt = Date.now();
      if (this.intervalId) {
        clearInterval(this.intervalId);
      }
      this.intervalId = setInterval(() => {
        if (this.slaRemaining <= 0 && this.intervalId) {
          clearInterval(this.intervalId);
          this.intervalId = null;
        }
      }, 1000);
    },
    stopSLAClock() {
      if (this.intervalId) {
        clearInterval(this.intervalId);
        this.intervalId = null;
      }
      this.slaStartedAt = null;
    },
    async submit() {
      const api = useKnowledgeSpaces();
      this.loading = true;
      this.error = null;
      try {
        const payload: KnowledgeSpacePayload = {
          ...this.form,
          requestedBy: this.iamEmail || "ops@powerx.local",
        };
        const response = await api.createSpace(payload);
        this.lastSpace = response;
        this.status = "success";
        this.startSLAClock();
        if (response?.spaceId && this.sampleDoc.enabled && this.sampleDoc.sourceUri) {
          try {
            this.lastIngestionJob = await api.triggerIngestion(response.spaceId, {
              format: this.sampleDoc.format,
              sourceUri: this.sampleDoc.sourceUri,
              ocrRequired: this.sampleDoc.ocrRequired,
              priority: "normal",
            });
          } catch (e) {
            console.warn("sample ingestion failed", e);
          }
        }

        if (this.runCorpusCheckAfterCreate && response?.spaceId) {
          try {
            this.lastCorpusCheckJob = await api.startCorpusCheck(
              response.spaceId,
              this.iamEmail || "ops@powerx.local",
            );
            for (let i = 0; i < 12; i++) {
              if (!this.lastCorpusCheckJob?.uuid) break;
              const latest = await api.getCorpusCheckJob(
                response.spaceId,
                this.lastCorpusCheckJob.uuid,
              );
              this.lastCorpusCheckJob = latest;
              if (latest?.status === "completed" || latest?.status === "failed") break;
              await new Promise((r) => setTimeout(r, 1000));
            }
          } catch (e) {
            console.warn("start corpus check failed", e);
          }
        }
      } catch (err) {
        this.error =
          err instanceof Error ? err.message : "提交失败，请稍后再试。";
        this.status = "error";
        throw err;
      } finally {
        this.loading = false;
      }
    },
  },
});
