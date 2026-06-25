<template>
  <div class="p-6 space-y-6">
    <div class="flex items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-semibold">团队管理</h1>
        <p class="text-sm text-gray-500">
          主页面只保留团队列表。点击“创建团队”后在弹层里完成成员选择和参数设置。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <UButton size="sm" variant="soft" icon="i-heroicons-chat-bubble-left-right" :to="localePath('/agent/sessions')">
          智能会话
        </UButton>
        <UButton size="sm" variant="soft" icon="i-heroicons-queue-list" :to="localePath('/agent/team-tasks')">
          团队任务
        </UButton>
        <UButton size="sm" color="primary" icon="i-heroicons-plus" @click="openCreateModal">
          创建团队
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="message"
      color="neutral"
      variant="soft"
      icon="i-heroicons-information-circle"
      :title="message"
    />

    <UCard>
      <template #header>
        <div class="flex flex-wrap items-center gap-3">
          <USelect
            v-model="parentAgentId"
            class="w-72"
            :items="parentFilterOptions"
            option-attribute="label"
            value-attribute="value"
          />
          <UCheckbox v-model="includeDisabled" label="包含 disabled" @change="loadTeams" />
          <UButton variant="soft" icon="i-heroicons-arrow-path" :loading="loading" @click="loadTeams">
            刷新列表
          </UButton>
        </div>
      </template>

      <UTable :data="teams" :columns="columns" row-key="id" />
    </UCard>

    <UModal
      v-model:open="editOpen"
      :title="`编辑团队 · ${editingTeamForm.team_name || '未命名团队'}`"
      description="可编辑 TL、团队名称、分发模式、失败策略。"
      :ui="{ content: 'max-w-2xl' }"
    >
      <template #body>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <UFormField label="TL（主智能体）">
            <USelect
              v-model="editingTeamForm.parent_agent_id"
              :items="agentOptions"
              option-attribute="label"
              value-attribute="value"
            />
          </UFormField>
          <UFormField label="团队名称">
            <UInput v-model="editingTeamForm.team_name" />
          </UFormField>
          <UFormField label="分发模式">
            <USelect v-model="editingTeamForm.dispatch_mode" :items="dispatchModeOptions" />
          </UFormField>
          <UFormField label="失败策略">
            <USelect v-model="editingTeamForm.default_failure_policy" :items="failurePolicyOptions" />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full items-center justify-end gap-2">
          <UButton variant="ghost" @click="editOpen = false">取消</UButton>
          <UButton color="primary" :loading="editSubmitting" @click="submitTeamEdit">保存</UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="memberOpen"
      :title="`成员管理 · ${editingTeam?.team_name || '未命名团队'}`"
      description="支持新增/更新/删除子智能体成员。主智能体（TL）请通过重建团队调整。"
      :ui="{ content: 'max-w-5xl' }"
    >
      <template #body>
        <div class="space-y-4">
          <div class="rounded-md border border-gray-200 p-3 text-xs text-gray-500">
            团队：{{ editingTeam?.team_name || "-" }} · TL：{{ resolveAgentProfile(editingTeam?.parent_agent_id || 0).title }} ·
            Mode：{{ editingTeam?.dispatch_mode || "-" }} · Failure：{{ editingTeam?.default_failure_policy || "-" }}
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-12">
            <UFormField label="子智能体" class="md:col-span-5">
              <USelect
                v-model="newMemberAgentId"
                :items="memberCandidateOptions"
                option-attribute="label"
                value-attribute="value"
                placeholder="请选择子智能体"
                :disabled="memberCandidateOptions.length === 0"
                class="w-full"
              />
            </UFormField>
            <UFormField label="角色" class="md:col-span-3">
              <USelect
                v-model="newMemberRole"
                :items="roleOptions"
                option-attribute="label"
                value-attribute="value"
                class="w-full"
              />
            </UFormField>
            <div class="flex items-end md:col-span-4">
              <UButton
                block
                color="primary"
                icon="i-heroicons-plus"
                :loading="memberSubmitting"
                @click="addOrUpdateMember"
              >
                新增/更新
              </UButton>
            </div>
          </div>
          <UAlert
            color="neutral"
            variant="soft"
            icon="i-heroicons-information-circle"
            :title="t('agent.teamManagement.roleGuide.title')"
            :description="t('agent.teamManagement.roleGuide.description')"
          />
          <UAlert
            v-if="memberCandidateOptions.length === 0"
            color="warning"
            variant="soft"
            icon="i-heroicons-exclamation-triangle"
            :title="t('agent.teamManagement.noChildCandidates.title')"
            :description="t('agent.teamManagement.noChildCandidates.description')"
          />

          <div class="rounded-lg border border-gray-200 overflow-hidden">
            <table class="w-full text-sm">
              <thead class="bg-gray-50">
                <tr>
                  <th class="px-3 py-2 text-left">Agent</th>
                  <th class="px-3 py-2 text-left">Role</th>
                  <th class="px-3 py-2 text-left">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in editingMembers" :key="item.child_agent_id" class="border-t border-gray-100">
                  <td class="px-3 py-2">
                    <div class="font-medium">{{ item.name || "未知智能体" }}</div>
                    <div class="text-xs text-gray-500">{{ item.key || "无 Key" }}</div>
                    <div class="text-[11px] text-gray-400">ID: {{ item.child_agent_id }}</div>
                  </td>
                  <td class="px-3 py-2">
                    <USelect
                      v-model="item.role"
                      :items="roleOptions"
                      option-attribute="label"
                      value-attribute="value"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-2">
                      <UButton size="xs" variant="soft" :loading="memberSubmitting" @click="saveMember(item)">
                        保存
                      </UButton>
                      <UButton size="xs" color="error" variant="outline" :loading="memberSubmitting" @click="removeMemberFromTeam(item.child_agent_id)">
                        删除
                      </UButton>
                    </div>
                  </td>
                </tr>
                <tr v-if="!editingMembers.length">
                  <td colspan="3" class="px-3 py-6 text-center text-gray-500">暂无子智能体成员</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>

      <template #footer>
        <div class="flex w-full justify-between items-center">
          <div class="text-xs text-gray-500">
            团队基础信息请在“编辑”操作中维护，成员增删改在此弹窗完成。
          </div>
          <UButton variant="ghost" @click="memberOpen = false">关闭</UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="createOpen"
      title="创建团队"
      description="先选择智能体，再指定 TL（团队负责人）。TL 会作为 parent_agent_id 写入。"
      :ui="{ content: 'max-w-6xl' }"
    >
      <template #body>
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div class="space-y-3">
            <UInput v-model="searchKeyword" icon="i-heroicons-magnifying-glass" placeholder="搜索智能体名称 / key" />
            <div class="rounded-lg border border-gray-200 p-3 max-h-[420px] overflow-auto space-y-2">
              <div
                v-for="agent in filteredAgents"
                :key="agent.id"
                class="flex items-center justify-between rounded-md border border-gray-200 px-3 py-2"
              >
                <div class="min-w-0">
                  <div class="font-medium truncate">{{ agent.name }}</div>
                  <div class="text-xs text-gray-500 truncate">{{ agent.key || "无 Key" }}</div>
                  <div class="text-[11px] text-gray-400 truncate">ID: {{ agent.id }}</div>
                </div>
                <UButton
                  size="xs"
                  icon="i-heroicons-plus"
                  :disabled="selectedAgentIds.has(agent.id)"
                  @click="addMember(agent)"
                >
                  添加
                </UButton>
              </div>
              <div v-if="filteredAgents.length === 0" class="text-sm text-gray-500">没有匹配的智能体</div>
            </div>
          </div>

          <div class="space-y-3">
            <div class="rounded-lg border border-gray-200 p-3 max-h-[420px] overflow-auto space-y-2">
              <div class="text-sm text-gray-500">已选成员（{{ selectedMembers.length }}）</div>
              <div
                v-for="member in selectedMembers"
                :key="member.agentId"
                class="rounded-md border border-gray-200 px-3 py-2 space-y-2"
              >
                <div class="flex items-center justify-between gap-2">
                  <div>
                    <div class="font-medium">{{ member.name }}</div>
                    <div class="text-xs text-gray-500">{{ member.key || "无 Key" }}</div>
                    <div class="text-[11px] text-gray-400">ID: {{ member.agentId }}</div>
                  </div>
                  <UButton size="xs" variant="ghost" color="error" icon="i-heroicons-trash" @click="removeMember(member.agentId)">
                    移除
                  </UButton>
                </div>
                <div class="grid grid-cols-1 gap-2 md:grid-cols-2">
                  <UFormField label="角色">
                    <USelect
                      v-model="member.role"
                      :items="roleOptions"
                      option-attribute="label"
                      value-attribute="value"
                      :disabled="tlAgentId === member.agentId"
                    />
                    <div v-if="tlAgentId === member.agentId" class="mt-1 text-xs text-gray-500">
                      {{ t("agent.teamManagement.create.tlFixedRoleHint") }}
                    </div>
                  </UFormField>
                  <UFormField label="TL">
                    <UButton
                      block
                      size="sm"
                      :variant="tlAgentId === member.agentId ? 'solid' : 'outline'"
                      :color="tlAgentId === member.agentId ? 'primary' : 'neutral'"
                      @click="tlAgentId = member.agentId"
                    >
                      {{ tlAgentId === member.agentId ? '当前 TL' : '设为 TL' }}
                    </UButton>
                  </UFormField>
                </div>
              </div>
              <div v-if="selectedMembers.length === 0" class="text-sm text-gray-500">请先从左侧添加至少一个智能体</div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
              <UFormField label="团队名称">
                <UInput v-model="teamName" placeholder="例如 incident-a2a-demo" />
              </UFormField>
              <UFormField label="分发模式">
                <USelect v-model="dispatchMode" :items="dispatchModeOptions" />
              </UFormField>
              <UFormField label="失败策略">
                <USelect v-model="failurePolicy" :items="failurePolicyOptions" />
              </UFormField>
            </div>
          </div>
        </div>
      </template>

      <template #footer>
        <div class="flex w-full items-center justify-between gap-2">
          <div class="text-xs text-gray-500">
            提示：TL 会作为主智能体写入，其他成员将 upsert 到 team members。
          </div>
          <div class="flex items-center gap-2">
            <UButton variant="ghost" @click="createOpen = false">取消</UButton>
            <UButton color="primary" :loading="creating" @click="createTeamFromModal">创建团队</UButton>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { h, resolveComponent } from "vue";
import { useAgentManager } from "~/composables/agent/useAgentManager";
import { useAgentTeamService, type AgentTeamRecord } from "~/composables/api/services/agentTeamService";
import type { Agent } from "~/types/agent";

const localePath = useLocalePath();
const { t } = useI18n();

definePageMeta({
  title: "团队管理",
  layout: "default",
});

const svc = useAgentTeamService();
const { agents, fetchAgents } = useAgentManager();
const agentCatalog = ref<Agent[]>([]);

const teams = ref<AgentTeamRecord[]>([]);
const parentAgentId = ref<number>(0);
const includeDisabled = ref(false);
const loading = ref(false);
const creating = ref(false);
const message = ref("");

const createOpen = ref(false);
const memberOpen = ref(false);
const editOpen = ref(false);
const searchKeyword = ref("");
const teamName = ref("incident-a2a-demo");
const dispatchMode = ref<"parallel" | "serial" | "mixed">("parallel");
const failurePolicy = ref<"continue" | "fail-fast" | "retry-once">("continue");
const tlAgentId = ref<number | null>(null);
type ChildRole = string;
const selectedMembers = ref<Array<{ agentId: number; name: string; key: string; role: ChildRole }>>([]);
const editingTeam = ref<AgentTeamRecord | null>(null);
const editingMembers = ref<Array<{ child_agent_id: number; role: ChildRole; enabled: boolean; name: string; key: string }>>([]);
const memberSubmitting = ref(false);
const editSubmitting = ref(false);
const newMemberAgentId = ref<number | null>(null);
const newMemberRole = ref<ChildRole>("executor");
const editingTeamForm = ref<{
  id: number;
  parent_agent_id: number;
  team_name: string;
  dispatch_mode: "parallel" | "serial" | "mixed";
  default_failure_policy: "continue" | "fail-fast" | "retry-once";
}>({
  id: 0,
  parent_agent_id: 0,
  team_name: "",
  dispatch_mode: "parallel",
  default_failure_policy: "continue",
});

const roleOptions = [
  { label: "资料检索 (retriever)", value: "retriever" },
  { label: "任务执行 (executor)", value: "executor" },
  { label: "结果复核 (reviewer)", value: "reviewer" },
];
const dispatchModeOptions = ["parallel", "serial", "mixed"];
const failurePolicyOptions = ["continue", "fail-fast", "retry-once"];
const agentPool = computed<Agent[]>(() =>
  (agentCatalog.value && agentCatalog.value.length ? agentCatalog.value : ((agents.value || []) as Agent[]))
);
const agentOptions = computed(() =>
  agentPool.value.map((agent) => ({
    label: `${agent.name || "未命名"} (${agent.key || "无 Key"})`,
    value: Number(agent.id),
  }))
);
const parentFilterOptions = computed(() => [{ label: "全部主智能体", value: 0 }, ...agentOptions.value]);
const isEligibleChildAgent = (agent: Agent) => {
  const isSystemScope = String(agent.scope || "").toLowerCase() === "system";
  const isBuiltin = Boolean((agent.meta as any)?.builtin || (agent.meta as any)?.protect_from_delete);
  const isActive = String(agent.status || "").toLowerCase() === "active";
  return !isSystemScope && !isBuiltin && isActive;
};
const eligibleChildAgents = computed(() => agentPool.value.filter((agent) => isEligibleChildAgent(agent)));
const memberCandidateOptions = computed(() => {
  const currentTL = Number(editingTeam.value?.parent_agent_id || 0);
  const list = eligibleChildAgents.value.filter((agent) => Number(agent.id || 0) !== currentTL);
  return list.map((agent) => ({
    label: `${agent.name || "未命名"} (${agent.key || "无 Key"})`,
    value: Number(agent.id),
  }));
});

const selectedAgentIds = computed(() => new Set(selectedMembers.value.map((item) => item.agentId)));
const filteredAgents = computed(() => {
  const q = searchKeyword.value.trim().toLowerCase();
  const list = eligibleChildAgents.value;
  if (!q) return list;
  return list.filter((agent) => {
    const name = String(agent.name || "").toLowerCase();
    const key = String(agent.key || "").toLowerCase();
    return name.includes(q) || key.includes(q) || String(agent.id).includes(q);
  });
});

const columns = [
  {
    accessorKey: "team_name",
    header: "Team",
    cell: ({ row }: any) => {
      const id = Number(row.original.id || 0);
      return h("div", { class: "space-y-0.5" }, [
        h("div", { class: "font-medium text-gray-900" }, row.original.team_name || "未命名团队"),
        h("div", { class: "text-xs text-gray-500" }, `ID: ${id}`),
      ]);
    },
  },
  {
    id: "parent",
    accessorKey: "parent_agent_id",
    header: "TL(主智能体)",
    cell: ({ row }: any) => {
      const id = Number(row.original.parent_agent_id || 0);
      const profile = resolveAgentProfile(id);
      return h("div", { class: "space-y-0.5" }, [
        h("div", { class: "font-medium text-gray-900" }, profile.name),
        h("div", { class: "text-xs text-gray-500" }, profile.key),
        h("div", { class: "text-[11px] text-gray-400" }, `ID: ${id}`),
      ]);
    },
  },
  { accessorKey: "dispatch_mode", header: "Mode" },
  { accessorKey: "default_failure_policy", header: "Failure" },
  {
    id: "status",
    accessorKey: "status",
    header: "Status",
    cell: ({ row }: any) =>
      h(
        resolveComponent("UButton"),
        {
          size: "xs",
          variant: "soft",
          color: row.original.status === "active" ? "success" : "neutral",
          onClick: () => toggleStatus(row.original as AgentTeamRecord),
        },
        () => row.original.status
      ),
  },
  {
    id: "actions",
    header: "操作",
    cell: ({ row }: any) =>
      h("div", { class: "flex items-center gap-2" }, [
        h(
          resolveComponent("UButton"),
          {
            size: "xs",
            variant: "outline",
            onClick: () =>
              navigateTo(
                `${localePath("/agent")}?workspace=team&team_id=${row.original.id}&parent_agent_id=${row.original.parent_agent_id}`
              ),
          },
          () => "进入任务"
        ),
        h(
          resolveComponent("UButton"),
          {
            size: "xs",
            variant: "soft",
            onClick: () => openMemberModal(row.original as AgentTeamRecord),
          },
          () => "成员管理"
        ),
        h(
          resolveComponent("UButton"),
          {
            size: "xs",
            variant: "soft",
            color: "neutral",
            onClick: () => openEditModal(row.original as AgentTeamRecord),
          },
          () => "编辑"
        ),
        h(
          resolveComponent("UButton"),
          {
            size: "xs",
            variant: "outline",
            color: "error",
            onClick: () => deleteTeam(row.original as AgentTeamRecord),
          },
          () => "删除"
        ),
      ]),
  },
];

const resetModalForm = () => {
  searchKeyword.value = "";
  selectedMembers.value = [];
  tlAgentId.value = null;
};

const openCreateModal = async () => {
  createOpen.value = true;
  await loadAgentCatalog();
};

const resolveAgentProfile = (agentId: number) => {
  const item = agentPool.value.find((agent) => Number(agent.id) === Number(agentId));
  if (!item) {
    return {
      name: `智能体#${agentId}`,
      key: "无 Key",
      title: `智能体#${agentId}`,
    };
  }
  return {
    name: item.name || `智能体#${agentId}`,
    key: item.key || "无 Key",
    title: `${item.name || `智能体#${agentId}`} (${item.key || "无 Key"})`,
  };
};

const loadAgentCatalog = async () => {
  await fetchAgents();
  agentCatalog.value = ((agents.value || []) as Agent[]).slice();
};

const loadMembersForModal = async () => {
  const teamId = Number(editingTeam.value?.id || 0);
  if (!teamId) return;
  try {
    const res = await svc.listMembers(teamId);
    editingMembers.value = (res.items || []).map((item) => ({
      child_agent_id: Number(item.child_agent_id),
      role: roleOptions.some((role) => role.value === item.role) ? item.role : "executor",
      enabled: Boolean(item.enabled),
      name: resolveAgentProfile(Number(item.child_agent_id)).name,
      key: resolveAgentProfile(Number(item.child_agent_id)).key,
    }));
  } catch (e: any) {
    message.value = e?.message || "加载成员失败";
    editingMembers.value = [];
  }
};

const openMemberModal = async (team: AgentTeamRecord) => {
  editingTeam.value = team;
  memberOpen.value = true;
  await loadAgentCatalog();
  await loadMembersForModal();
};

const openEditModal = (team: AgentTeamRecord) => {
  editingTeamForm.value = {
    id: Number(team.id),
    parent_agent_id: Number(team.parent_agent_id),
    team_name: String(team.team_name || ""),
    dispatch_mode: team.dispatch_mode,
    default_failure_policy: team.default_failure_policy,
  };
  editOpen.value = true;
};

const submitTeamEdit = async () => {
  const teamId = Number(editingTeamForm.value.id || 0);
  if (!teamId) return;
  if (!editingTeamForm.value.parent_agent_id || editingTeamForm.value.parent_agent_id <= 0) {
    message.value = "请选择 TL（主智能体）";
    return;
  }
  if (!editingTeamForm.value.team_name.trim()) {
    message.value = "团队名称不能为空";
    return;
  }
  editSubmitting.value = true;
  try {
    const updated = await svc.updateTeam(teamId, {
      parent_agent_id: Number(editingTeamForm.value.parent_agent_id),
      team_name: editingTeamForm.value.team_name.trim(),
      dispatch_mode: editingTeamForm.value.dispatch_mode,
      default_failure_policy: editingTeamForm.value.default_failure_policy,
    });
    message.value = `团队已更新：${updated.team_name}`;
    editOpen.value = false;
    parentAgentId.value = Number(updated.parent_agent_id);
    await loadTeams();
  } catch (e: any) {
    message.value = e?.message || "更新团队失败";
  } finally {
    editSubmitting.value = false;
  }
};

const deleteTeam = async (team: AgentTeamRecord) => {
  const ok = window.confirm(`确认删除团队 #${team.id}（${team.team_name}）？此操作会删除成员绑定与handoff记录。`);
  if (!ok) return;
  try {
    await svc.deleteTeam(Number(team.id));
    message.value = `团队已删除：${team.team_name}`;
    if (Number(parentAgentId.value) === Number(team.parent_agent_id)) {
      await loadTeams();
      return;
    }
    parentAgentId.value = Number(team.parent_agent_id);
    await loadTeams();
  } catch (e: any) {
    message.value = e?.message || "删除团队失败";
  }
};

const addOrUpdateMember = async () => {
  const teamId = Number(editingTeam.value?.id || 0);
  const childAgentID = Number(newMemberAgentId.value || 0);
  if (!teamId) return;
  if (!childAgentID || childAgentID <= 0) {
    message.value = "请选择子智能体";
    return;
  }
  if (childAgentID === Number(editingTeam.value?.parent_agent_id || 0)) {
    message.value = "子智能体不能与 TL 相同";
    return;
  }
  memberSubmitting.value = true;
  try {
    await svc.upsertMember(teamId, {
      child_agent_id: childAgentID,
      role: newMemberRole.value,
      priority: 1,
      enabled: true,
    });
    message.value = `成员已写入：${resolveAgentProfile(childAgentID).title}`;
    await loadMembersForModal();
  } catch (e: any) {
    message.value = e?.message || "写入成员失败";
  } finally {
    memberSubmitting.value = false;
  }
};

const saveMember = async (item: { child_agent_id: number; role: ChildRole }) => {
  const teamId = Number(editingTeam.value?.id || 0);
  if (!teamId) return;
  memberSubmitting.value = true;
  try {
    await svc.upsertMember(teamId, {
      child_agent_id: Number(item.child_agent_id),
      role: item.role,
      priority: 1,
      enabled: true,
    });
    message.value = `成员已更新：${resolveAgentProfile(item.child_agent_id).title}`;
    await loadMembersForModal();
  } catch (e: any) {
    message.value = e?.message || "更新成员失败";
  } finally {
    memberSubmitting.value = false;
  }
};

const removeMemberFromTeam = async (childAgentID: number) => {
  const teamId = Number(editingTeam.value?.id || 0);
  if (!teamId) return;
  memberSubmitting.value = true;
  try {
    await svc.deleteMember(teamId, childAgentID);
    message.value = `成员已删除：${resolveAgentProfile(childAgentID).title}`;
    await loadMembersForModal();
  } catch (e: any) {
    message.value = e?.message || "删除成员失败";
  } finally {
    memberSubmitting.value = false;
  }
};

const addMember = (agent: Agent) => {
  if (selectedAgentIds.value.has(agent.id)) return;
  selectedMembers.value.push({
    agentId: agent.id,
    name: agent.name,
    key: agent.key || "",
    role: "executor",
  });
  if (!tlAgentId.value) tlAgentId.value = agent.id;
};

const removeMember = (agentId: number) => {
  selectedMembers.value = selectedMembers.value.filter((item) => item.agentId !== agentId);
  if (tlAgentId.value === agentId) {
    tlAgentId.value = selectedMembers.value[0]?.agentId ?? null;
  }
};

const loadTeams = async () => {
  loading.value = true;
  try {
    const byParent = parentAgentId.value > 0 ? parentAgentId.value : undefined;
    const res = await svc.listTeams(byParent, includeDisabled.value);
    teams.value = res.items ?? [];
    message.value = `已加载 ${teams.value.length} 条团队记录`;
  } catch (e: any) {
    message.value = e?.message || "加载失败";
  } finally {
    loading.value = false;
  }
};

const createTeamFromModal = async () => {
  if (!teamName.value.trim()) {
    message.value = "请填写团队名称";
    return;
  }
  if (!selectedMembers.value.length) {
    message.value = "请至少添加一个智能体";
    return;
  }
  if (!tlAgentId.value) {
    message.value = "请指定 TL（主智能体）";
    return;
  }

  creating.value = true;
  try {
    const created = await svc.createTeam({
      parent_agent_id: tlAgentId.value,
      team_name: teamName.value.trim(),
      dispatch_mode: dispatchMode.value,
      default_failure_policy: failurePolicy.value,
    });

    const children = selectedMembers.value.filter((item) => item.agentId !== tlAgentId.value);
    for (const member of children) {
      await svc.upsertMember(created.id, {
        child_agent_id: member.agentId,
        role: member.role,
        priority: 1,
        enabled: true,
      });
    }

    parentAgentId.value = created.parent_agent_id;
    message.value = `创建成功：${created.team_name}，成员写入 ${children.length} 个`;
    createOpen.value = false;
    resetModalForm();
    await loadTeams();
  } catch (e: any) {
    message.value = e?.message || "创建失败";
  } finally {
    creating.value = false;
  }
};

const toggleStatus = async (item: AgentTeamRecord) => {
  const nextStatus = item.status === "active" ? "disabled" : "active";
  try {
    await svc.updateTeamStatus(item.id, nextStatus);
    await loadTeams();
  } catch (e: any) {
    message.value = e?.message || "更新状态失败";
  }
};

watch(includeDisabled, () => {
  void loadTeams();
});

onMounted(async () => {
  await Promise.all([loadAgentCatalog(), loadTeams()]);
});
</script>
