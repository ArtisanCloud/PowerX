<script setup lang="ts">
import { reactive, ref } from "vue";
import type {
  FusionStrategyPayload,
  FusionStrategyRecord,
} from "~/composables/useKnowledgeSpaces";
import { useKnowledgeSpaces } from "~/composables/useKnowledgeSpaces";

useHead({
  title: "融合策略管理",
});

const api = useKnowledgeSpaces();
const spaceId = ref("");
const loadingList = ref(false);
const submitting = ref(false);
const statusMessage = ref("");
const errorMessage = ref("");
const strategies = ref<FusionStrategyRecord[]>([]);

const publishForm = reactive<FusionStrategyPayload>({
  label: "",
  bm25Weight: 0.5,
  vectorWeight: 0.5,
  graphConstraint: "",
  rerankerModel: "cross-encoder-v1",
  conflictPolicy: "allow_with_flag",
});

const loadStrategies = async () => {
  if (!spaceId.value) {
    errorMessage.value = "请先输入空间 ID";
    return;
  }
  errorMessage.value = "";
  loadingList.value = true;
  try {
    strategies.value = await api.listFusionStrategies(spaceId.value);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    errorMessage.value = `加载策略失败：${message}`;
  } finally {
    loadingList.value = false;
  }
};

const publishStrategy = async () => {
  if (!spaceId.value) {
    errorMessage.value = "请先输入空间 ID";
    return;
  }
  errorMessage.value = "";
  statusMessage.value = "";
  submitting.value = true;
  try {
    const result = await api.publishFusionStrategy(
      spaceId.value,
      publishForm,
    );
    statusMessage.value =
      result.deploymentState === "draft"
        ? "策略已排队等待发布"
        : "策略发布成功";
    strategies.value = [result, ...strategies.value];
    publishForm.label = "";
    publishForm.graphConstraint = "";
  } catch (error) {
    const message = error instanceof Error ? error.message : "发布失败";
    errorMessage.value = message;
  } finally {
    submitting.value = false;
  }
};

const rollbackStrategy = async (strategy: FusionStrategyRecord) => {
  if (!spaceId.value) {
    return;
  }
  errorMessage.value = "";
  statusMessage.value = "";
  try {
    const result = await api.rollbackFusionStrategy(
      spaceId.value,
      strategy.strategyId,
    );
    statusMessage.value = "回滚已触发";
    strategies.value = strategies.value.map(current =>
      current.strategyId === result.strategyId ? result : current,
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : "回滚失败";
    errorMessage.value = message;
  }
};
</script>

<template>
  <section class="px-6 py-8 space-y-8">
    <header class="space-y-2">
      <p class="text-sm text-gray-500">Fusion Strategies</p>
      <h1 class="text-2xl font-semibold text-gray-900">融合策略管理</h1>
      <p class="text-gray-600">
        管理 BM25 / 向量 / 图谱权重，监控冲突策略并快速回滚。
      </p>
    </header>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">空间与策略</h2>
            <p class="text-sm text-gray-500">选择空间并加载历史策略。</p>
          </div>
        </div>
      </template>
      <div class="flex flex-col gap-4 md:flex-row md:items-end">
        <label class="flex-1 text-sm font-medium text-gray-700">
          空间 ID
          <input
            v-model="spaceId"
            type="text"
            class="mt-1 w-full rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
            placeholder="a25b4e6a-..."
          />
        </label>
        <UButton
          :loading="loadingList"
          @click="loadStrategies"
        >
          加载策略
        </UButton>
      </div>

      <div v-if="strategies.length" class="mt-6 space-y-3">
        <div
          v-for="strategy in strategies"
          :key="strategy.strategyId"
          class="flex flex-col gap-2 rounded-lg border border-gray-200 p-4 md:flex-row md:items-center md:justify-between"
        >
          <div>
            <p class="text-sm font-medium text-gray-900">
              {{ strategy.label }} · {{ strategy.strategyId }}
            </p>
            <p class="text-xs text-gray-500">
              BM25 {{ strategy.bm25Weight }} ｜ 向量
              {{ strategy.vectorWeight }} ｜ 模型 {{ strategy.rerankerModel }}
            </p>
            <p class="text-xs text-gray-500">
              状态：{{ strategy.deploymentState }} ｜ 冲突策略
              {{ strategy.conflictPolicy }}
            </p>
          </div>
          <UButton
            color="primary"
            variant="soft"
            @click="rollbackStrategy(strategy)"
          >
            回滚 {{ strategy.label }}
          </UButton>
        </div>
      </div>
      <p v-else class="mt-6 text-sm text-gray-500">
        尚未加载策略或该空间暂无版本。
      </p>
    </UCard>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">发布策略</h2>
            <p class="text-sm text-gray-500">
              配置权重与冲突策略，支持排队或立即生效。
            </p>
          </div>
        </div>
      </template>
      <div class="grid gap-4 md:grid-cols-2">
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">策略名称</span>
          <input
            v-model="publishForm.label"
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">BM25 权重</span>
          <input
            v-model.number="publishForm.bm25Weight"
            type="number"
            step="0.1"
            min="0"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">向量权重</span>
          <input
            v-model.number="publishForm.vectorWeight"
            type="number"
            step="0.1"
            min="0"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2 md:col-span-2">
          <span class="text-sm font-medium text-gray-700">图谱约束</span>
          <input
            v-model="publishForm.graphConstraint"
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">Reranker 模型</span>
          <input
            v-model="publishForm.rerankerModel"
            type="text"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          />
        </label>
        <label class="flex flex-col gap-2">
          <span class="text-sm font-medium text-gray-700">冲突策略</span>
          <select
            v-model="publishForm.conflictPolicy"
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          >
            <option value="allow_with_flag">立即发布（allow_with_flag）</option>
            <option value="queue">排队（queue）</option>
            <option value="block">阻断（block）</option>
          </select>
        </label>
      </div>
      <div class="mt-4 flex items-center gap-3">
        <UButton :loading="submitting" @click="publishStrategy">
          发布策略
        </UButton>
        <p v-if="statusMessage" class="text-sm text-primary-600">
          {{ statusMessage }}
        </p>
        <p v-if="errorMessage" class="text-sm text-red-500">
          {{ errorMessage }}
        </p>
      </div>
    </UCard>
  </section>
</template>
