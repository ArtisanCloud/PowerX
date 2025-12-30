<script setup lang="ts">
import type { KnowledgeSpacePayload } from "~/app/composables/useKnowledgeSpaces";

const props = defineProps<{
  payload: KnowledgeSpacePayload;
  iamEmail: string;
  slaRemaining: number;
}>();
</script>

<template>
  <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
    <div class="mb-4 flex items-center justify-between">
      <div>
        <p class="text-sm uppercase tracking-wide text-gray-500">
          审计预览
        </p>
        <h3 class="text-lg font-semibold text-gray-900">
          {{ payload.spaceName || "待确认空间" }}
        </h3>
      </div>
      <UBadge color="primary" variant="soft">SLA 计时：{{ slaRemaining }}s</UBadge>
    </div>

    <dl class="grid gap-4 md:grid-cols-2">
      <div>
        <dt class="text-xs uppercase text-gray-500">租户 UUID</dt>
        <dd class="text-sm text-gray-900">{{ payload.tenantUuid || "未填写" }}</dd>
      </div>
      <div>
        <dt class="text-xs uppercase text-gray-500">部门编码</dt>
        <dd class="text-sm text-gray-900">
          {{ payload.departmentCode || "未填写" }}
        </dd>
      </div>
      <div>
        <dt class="text-xs uppercase text-gray-500">策略模版</dt>
        <dd class="text-sm text-gray-900">
          {{ payload.policyTemplateVersionId }}
        </dd>
      </div>
      <div>
        <dt class="text-xs uppercase text-gray-500">IAM 通知</dt>
        <dd class="text-sm text-gray-900">{{ iamEmail || "未设置" }}</dd>
      </div>
      <div>
        <dt class="text-xs uppercase text-gray-500">配额</dt>
        <dd class="text-sm text-gray-900">
          CPU {{ payload.quotas.cpuCores }} · 存储 {{ payload.quotas.storageGb }}GB
          · 并发 {{ payload.quotas.ingestionConcurrency }}
        </dd>
      </div>
      <div>
        <dt class="text-xs uppercase text-gray-500">特性</dt>
        <dd class="text-sm text-gray-900">
          <span v-if="payload.featureFlags.length === 0">默认配置</span>
          <UBadge
            v-for="flag in payload.featureFlags"
            :key="flag"
            color="gray"
            variant="soft"
            class="mr-1"
          >
            {{ flag }}
          </UBadge>
        </dd>
      </div>
    </dl>
  </div>
</template>
