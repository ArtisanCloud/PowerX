<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">智能体管理</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          统一管理单智能体，支持快速进入智能会话或创建团队。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
          进入智能会话
        </UButton>
        <UButton color="primary" icon="i-heroicons-plus" @click="createAgentQuick">
          新建智能体
        </UButton>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      <UCard v-for="agent in agents" :key="agent.uuid" class="border border-[var(--border-color)]">
        <div class="space-y-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="text-base font-medium text-[var(--text-primary)]">{{ agent.name }}</div>
              <div class="break-all text-xs text-[var(--text-secondary)]">{{ agent.key }}</div>
            </div>
            <UBadge class="shrink-0" :color="agent.status === 'active' ? 'success' : 'neutral'" variant="soft">
              {{ agent.status || 'unknown' }}
            </UBadge>
          </div>

          <p class="line-clamp-2 text-sm text-[var(--text-secondary)]">
            {{ agent.description || '暂无描述' }}
          </p>

          <div class="flex items-center gap-2">
            <UButton size="xs" variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
              智能会话
            </UButton>
            <UButton size="xs" variant="soft" icon="i-heroicons-user-group" :to="localePath('/settings/ai/agent-teams')">
              创建团队
            </UButton>
            <UButton size="xs" variant="ghost" icon="i-heroicons-pencil-square" @click="openEditForm(agent)">
              编辑
            </UButton>
          </div>
        </div>
      </UCard>
    </div>

    <UAlert
      v-if="errorText"
      color="error"
      variant="soft"
      icon="i-heroicons-exclamation-triangle"
      :title="errorText"
    />

    <UModal
      v-model:open="editOpen"
      title="编辑智能体"
      description="维护基础信息、生成策略与资源绑定"
      :ui="{ content: 'sm:max-w-5xl' }"
    >
      <template #body>
        <div class="pr-1">
          <div class="mx-auto max-w-4xl space-y-5">
            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">基础信息</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-5" label="智能体名称" required>
                  <UInput v-model="editForm.name" class="w-full" placeholder="请输入智能体名称" />
                </UFormField>
                <UFormField class="lg:col-span-3" label="类型标识（Type ID）">
                  <UInput v-model="editForm.typeId" class="w-full" placeholder="如 brand.story.v1" />
                </UFormField>
                <UFormField class="lg:col-span-2" label="场景（Scene）">
                  <UInput v-model="editForm.scene" class="w-full" placeholder="如 brand_story" />
                </UFormField>
                <UFormField class="lg:col-span-2" label="状态">
                  <USelect v-model="editForm.status" class="w-full" :items="statusItems" />
                </UFormField>
                <UFormField class="lg:col-span-12" label="对外简介（可选）" description="用于列表展示的简短说明，建议 30~80 字">
                  <UInput v-model="editForm.description" class="w-full" placeholder="例如：用于短视频口播脚本生成与投流建议" />
                </UFormField>
                <UFormField class="lg:col-span-12" label="系统 Key（只读）">
                  <UInput class="w-full" :model-value="editForm.key" disabled />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">生成策略</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-12" label="系统提示词（主指令）" description="必填主策略：任务目标、输出格式、约束边界都写在这里">
                  <UTextarea v-model="editForm.promptSeed" class="w-full min-h-56" :rows="8" placeholder="请输入主指令（建议包含目标、结构、边界、禁止项）" />
                </UFormField>
                <UFormField class="lg:col-span-12" label="角色设定（高级可选）" description="仅在你需要补充固定人设时填写；会在主指令之外作为补充上下文">
                  <UTextarea v-model="editForm.persona" class="w-full min-h-40" :rows="5" placeholder="可选：如未填写则仅使用系统提示词" />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="text-sm font-semibold text-[var(--text-primary)]">资源绑定</div>
              <div class="grid grid-cols-1 gap-4 lg:grid-cols-12">
                <UFormField class="lg:col-span-12" label="技能 IDs" description="使用英文逗号分隔">
                  <UInput v-model="editForm.skillIdsText" class="w-full" placeholder="如 skill.a,skill.b" />
                </UFormField>
                <UFormField class="lg:col-span-12" label="知识库 IDs" description="使用英文逗号分隔（UUID）">
                  <UInput v-model="editForm.knowledgeBaseIdsText" class="w-full" placeholder="如 uuid1,uuid2" />
                </UFormField>
              </div>
            </section>

            <section class="space-y-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-muted)]/20 p-4">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-sm font-semibold text-[var(--text-primary)]">
                    {{ t('agent.management.grants.title') }}
                  </div>
                  <p class="text-xs text-[var(--text-secondary)]">
                    {{ t('agent.management.grants.description') }}
                  </p>
                </div>
                <UBadge variant="soft" color="neutral">
                  {{ selectedGrantKeys.size }}/{{ grantableCapabilities.length }}
                </UBadge>
              </div>

              <UAlert
                v-if="grantError"
                color="error"
                variant="soft"
                icon="i-heroicons-exclamation-triangle"
                :title="grantError"
              />

              <div v-if="grantLoading" class="py-4 text-sm text-[var(--text-secondary)]">
                {{ t('agent.management.grants.loading') }}
              </div>
              <div v-else-if="grantableCapabilities.length === 0" class="py-4 text-sm text-[var(--text-secondary)]">
                {{ t('agent.management.grants.empty') }}
              </div>
              <div v-else class="max-h-80 space-y-2 overflow-y-auto pr-1">
                <div
                  v-for="item in grantableCapabilities"
                  :key="grantKey(item.capability_uuid, item.permission_code)"
                  class="flex items-start justify-between gap-4 rounded-md border border-[var(--border-color)] bg-[var(--bg-elevated)] p-3"
                >
                  <div class="min-w-0 space-y-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="font-medium text-[var(--text-primary)]">{{ item.display_name || item.capability_id }}</span>
                      <UBadge size="xs" variant="soft" color="neutral">{{ item.risk_level || 'unknown' }}</UBadge>
                      <UBadge
                        size="xs"
                        variant="soft"
                        :color="item.tenant_enabled && item.agent_usable ? 'success' : 'warning'"
                      >
                        {{ item.tenant_enabled && item.agent_usable ? t('agent.management.grants.available') : t('agent.management.grants.unavailable') }}
                      </UBadge>
                    </div>
                    <div class="break-all font-mono text-xs text-[var(--text-secondary)]">
                      {{ item.permission_code }}
                    </div>
                    <div class="break-all text-xs text-[var(--text-muted)]">
                      {{ item.plugin_id }} · {{ item.capability_id }}
                    </div>
                  </div>
                  <USwitch
                    :model-value="selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code))"
                    :disabled="!item.tenant_enabled || !item.agent_usable"
                    @update:model-value="setGrantSelected(item, Boolean($event))"
                  />
                </div>
              </div>
            </section>
          </div>
        </div>
      </template>
      <template #footer>
        <div class="sticky bottom-0 z-10 -mx-6 -mb-6 flex justify-end gap-2 border-t border-[var(--border-color)] bg-[var(--bg-elevated)] px-6 py-4">
          <UButton variant="soft" @click="editOpen = false">取消</UButton>
          <UButton color="primary" :loading="submitting" @click="submitEdit">保存</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { useAgentManager } from '~/composables/agent/useAgentManager'
import type { Agent, AgentGrantableCapability } from '~/types/agent'

const localePath = useLocalePath()
const { t } = useI18n()
const toast = useToast()
const {
  agents,
  fetchAgents,
  createAgent,
  updateAgent,
  fetchGrantableCapabilities,
  fetchAgentGrants,
  updateAgentGrants,
} = useAgentManager()

const errorText = ref('')
const editOpen = ref(false)
const submitting = ref(false)
const grantLoading = ref(false)
const grantError = ref('')
const grantableCapabilities = ref<AgentGrantableCapability[]>([])
const selectedGrantKeys = reactive(new Set<string>())
const editUUID = ref('')
const editForm = reactive({
  key: '',
  name: '',
  description: '',
  status: 'draft',
  typeId: '',
  scene: '',
  promptSeed: '',
  persona: '',
  skillIdsText: '',
  knowledgeBaseIdsText: '',
})
const statusItems = [
  { label: 'draft', value: 'draft' },
  { label: 'active', value: 'active' },
  { label: 'disabled', value: 'disabled' },
]

const load = async () => {
  try {
    errorText.value = ''
    await fetchAgents()
  } catch (e: any) {
    errorText.value = e?.message || '加载智能体失败'
  }
}

const createAgentQuick = async () => {
  const key = `agent_${Date.now().toString().slice(-6)}`
  try {
    await createAgent({
      key,
      name: `新智能体-${key.slice(-4)}`,
      description: '请在智能体管理中完善配置',
      status: 'active',
      meta: {},
    })
    toast.add({ title: '创建成功', description: '已创建默认智能体', color: 'success' })
    await load()
  } catch (e: any) {
    toast.add({ title: '创建失败', description: e?.message || '未知错误', color: 'error' })
  }
}

const openEditForm = async (agent: Agent) => {
  editUUID.value = agent.uuid
  editForm.key = agent.key || ''
  editForm.name = agent.name || ''
  editForm.description = agent.description || ''
  editForm.status = agent.status || 'draft'
  editForm.typeId = (agent as any).typeId || ''
  editForm.scene = (agent as any).scene || ''
  editForm.promptSeed = (agent as any).promptSeed || ''
  editForm.persona = (agent as any).persona || ''
  editForm.skillIdsText = normalizeArrayText((agent as any).skillIds)
  editForm.knowledgeBaseIdsText = normalizeArrayText((agent as any).knowledgeBaseIds)
  editOpen.value = true
  await loadAgentGrantState(agent.uuid)
}

const submitEdit = async () => {
  if (!editUUID.value) return
  if (!editForm.name.trim()) {
    toast.add({ title: '名称不能为空', color: 'warning' })
    return
  }
  try {
    submitting.value = true
    await updateAgent(editUUID.value, {
      name: editForm.name.trim(),
      description: editForm.description.trim(),
      status: editForm.status,
      typeId: editForm.typeId.trim(),
      scene: editForm.scene.trim(),
      promptSeed: editForm.promptSeed.trim(),
      persona: editForm.persona.trim(),
      skillIds: parseCommaValues(editForm.skillIdsText),
      knowledgeBaseIds: parseCommaValues(editForm.knowledgeBaseIdsText),
    })
    await updateAgentGrants(editUUID.value, grantPayload())
    toast.add({ title: '更新成功', color: 'success' })
    editOpen.value = false
    await load()
  } catch (e: any) {
    toast.add({ title: '更新失败', description: e?.message || '未知错误', color: 'error' })
  } finally {
    submitting.value = false
  }
}

const loadAgentGrantState = async (agentUUID: string) => {
  try {
    grantLoading.value = true
    grantError.value = ''
    selectedGrantKeys.clear()
    const [catalog, grants] = await Promise.all([
      fetchGrantableCapabilities(),
      fetchAgentGrants(agentUUID),
    ])
    grantableCapabilities.value = catalog
    for (const grant of grants) {
      if (grant.status === 'enabled') {
        selectedGrantKeys.add(grantKey(grant.capability_uuid, grant.permission_code))
      }
    }
  } catch (e: any) {
    grantError.value = e?.message || t('agent.management.grants.loadFailed')
  } finally {
    grantLoading.value = false
  }
}

const grantKey = (capabilityUUID: string, permissionCode: string) =>
  `${String(capabilityUUID || '').toLowerCase()}|${String(permissionCode || '').toLowerCase()}`

const setGrantSelected = (item: AgentGrantableCapability, selected: boolean) => {
  const key = grantKey(item.capability_uuid, item.permission_code)
  if (selected) selectedGrantKeys.add(key)
  else selectedGrantKeys.delete(key)
}

const grantPayload = () =>
  grantableCapabilities.value.map((item) => ({
    capability_uuid: item.capability_uuid,
    permission_code: item.permission_code,
    enabled: selectedGrantKeys.has(grantKey(item.capability_uuid, item.permission_code)),
  }))

onMounted(load)

const parseCommaValues = (raw: string): string[] => {
  if (!raw) return []
  const parts = raw.split(',').map((item) => item.trim()).filter(Boolean)
  return Array.from(new Set(parts))
}

const normalizeArrayText = (value: unknown): string => {
  if (!Array.isArray(value)) return ''
  return value.map((item) => String(item || '').trim()).filter(Boolean).join(',')
}
</script>
