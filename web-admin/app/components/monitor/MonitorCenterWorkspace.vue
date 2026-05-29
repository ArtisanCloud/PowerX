<template>
  <div class="p-6 space-y-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">监控中心</h1>
        <p class="text-gray-600 dark:text-gray-400">
          聚焦运行态 Queue 与联调，Topic 维护请前往事件管理页。
        </p>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      title="无权限"
      description="仅 Root 管理员可查看监控中心。"
    />

    <div v-else class="space-y-4">
      <UCard v-if="showTopTabs">
        <div class="flex flex-wrap gap-2">
          <UButton
            v-for="tab in monitorTabs"
            :key="tab.key"
            size="sm"
            :variant="activeTab === tab.key ? 'solid' : 'outline'"
            :color="activeTab === tab.key ? 'primary' : 'neutral'"
            @click="setTab(tab.key)"
          >
            {{ tab.label }}
          </UButton>
        </div>
      </UCard>

      <div v-if="activeTab === 'event-fabric'" class="space-y-4">
        <UCard>
          <template #header>
            <div class="flex flex-wrap items-center gap-3">
              <div class="text-sm text-gray-600 dark:text-gray-400">
                当前登录租户：
                <span class="font-mono">{{ effectiveTenantName }}</span>
              </div>
              <div class="text-sm text-gray-600 dark:text-gray-400">
                更新时间：
                <span class="font-mono">{{ overview?.now || '-' }}</span>
              </div>
            </div>
          </template>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-6">
            <UInput v-model="filters.namespace" icon="i-heroicons-tag" placeholder="事件域（例：_topic.knowledge.space.feedback）" />
            <UInput v-model="filters.name" icon="i-heroicons-hashtag" placeholder="事件名（例：reprocess）" />
            <UInput v-model="filters.subscriberId" icon="i-heroicons-identification" placeholder="订阅者ID（支持片段：llm_job_worker）" />
            <USelect
              v-model="selectedQueueScope"
              :items="queueScopeOptions"
              value-key="value"
              label-key="label"
              class="w-full"
            />
            <USelectMenu
              v-model="selectedQueueTenant"
              v-model:search-term="queueTenantSearch"
              :items="queueTenantOptions"
              searchable
              value-key="value"
              label-key="label"
              :disabled="selectedQueueScope === 'all'"
              :portal="false"
              :placeholder="queueTenantPlaceholder"
              class="w-full"
            />
            <UButton variant="outline" @click="refresh">应用筛选</UButton>
          </div>

          <div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500">
            <span>常用预设：</span>
            <UButton size="xs" variant="soft" @click="applyFilterPreset('feedback_reprocess')">反馈重处理</UButton>
            <UButton size="xs" variant="soft" @click="applyFilterPreset('corpus_check')">语料巡检</UButton>
            <UButton size="xs" variant="ghost" @click="applyFilterPreset('clear')">清空筛选</UButton>
          </div>

          <div class="mt-1 text-xs text-gray-500">
            说明：先选队列来源，再选对应租户。PowerX 底座队列按 PowerX 租户筛选；插件队列按插件上报的本地租户筛选。
          </div>
        </UCard>

        <div class="grid grid-cols-1 gap-4 lg:grid-cols-5">
          <UCard class="lg:col-span-1">
            <template #header>
              <div class="font-semibold">二级视图</div>
            </template>
            <div class="space-y-2">
              <UButton
                v-for="tab in eventSubTabs"
                :key="tab.key"
                block
                size="sm"
                :variant="eventSubTab === tab.key ? 'solid' : 'outline'"
                :color="eventSubTab === tab.key ? 'primary' : 'neutral'"
                @click="setEventSubTab(tab.key)"
              >
                {{ tab.label }}
              </UButton>
            </div>
            <div class="mt-3 text-xs text-gray-500 leading-5">
              建议顺序：先看 Queue，再看联调。
            </div>
          </UCard>

          <div class="space-y-4 lg:col-span-4">
        <UCard v-if="eventSubTab === 'queue'">
          <template #header>
            <div class="flex items-center justify-between gap-3">
              <div class="font-semibold">任务队列（统一机制）</div>
              <UButton size="xs" variant="outline" :loading="taskQueueLoading" @click="loadTaskQueueStats(true)">刷新队列统计</UButton>
            </div>
          </template>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
            <div :class="metricCardClass(taskQueueStats?.pending ?? 0, 'pending')">
              <div class="text-xs text-gray-500">待消费（pending）</div>
              <div :class="metricValueClass(taskQueueStats?.pending ?? 0, 'pending')">{{ taskQueueStats?.pending ?? 0 }}</div>
            </div>
            <div :class="metricCardClass(taskQueueStats?.deferred ?? 0, 'deferred')">
              <div class="text-xs text-gray-500">延迟队列（deferred）</div>
              <div :class="metricValueClass(taskQueueStats?.deferred ?? 0, 'deferred')">{{ taskQueueStats?.deferred ?? 0 }}</div>
            </div>
            <div :class="metricCardClass(taskQueueStats?.processing ?? 0, 'processing')">
              <div class="text-xs text-gray-500">处理中（processing）</div>
              <div :class="metricValueClass(taskQueueStats?.processing ?? 0, 'processing')">{{ taskQueueStats?.processing ?? 0 }}</div>
            </div>
            <div :class="metricCardClass(taskQueueStats?.inflight ?? 0, 'inflight')">
              <div class="text-xs text-gray-500">飞行中（inflight）</div>
              <div :class="metricValueClass(taskQueueStats?.inflight ?? 0, 'inflight')">{{ taskQueueStats?.inflight ?? 0 }}</div>
            </div>
          </div>

          <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-4">
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div class="text-xs text-gray-500">历史任务总数</div>
              <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ taskQueueHistorySummary.total }}</div>
            </div>
            <div class="rounded border border-emerald-200 dark:border-emerald-800 bg-emerald-50/50 dark:bg-emerald-950/20 p-3">
              <div class="text-xs text-gray-500">已完成</div>
              <div class="mt-1 text-xl font-semibold text-emerald-700 dark:text-emerald-300">{{ taskQueueHistorySummary.completed }}</div>
            </div>
            <div class="rounded border border-red-200 dark:border-red-800 bg-red-50/50 dark:bg-red-950/20 p-3">
              <div class="text-xs text-gray-500">失败</div>
              <div class="mt-1 text-xl font-semibold text-red-700 dark:text-red-300">{{ taskQueueHistorySummary.failed }}</div>
            </div>
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3">
              <div class="text-xs text-gray-500">最近执行</div>
              <div class="mt-1 text-sm font-mono text-gray-900 dark:text-white">{{ formatMonitorLogTs(taskQueueHistorySummary.lastSeenAt) }}</div>
            </div>
          </div>

          <div class="mt-3 text-xs text-gray-500">
            统计时间：<span class="font-mono">{{ taskQueueNow || '-' }}</span>
            <span class="ml-2">当前表为 subscriber + tenant_key 聚合视图；点击状态数字或“任务清单”进入该订阅分片的任务明细。</span>
          </div>

          <div class="mt-3 overflow-x-auto">
            <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
              <thead class="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th class="text-left px-3 py-2">subscriber_id</th>
                  <th class="text-left px-3 py-2">来源</th>
                  <th class="text-left px-3 py-2">PowerX 租户 / tenant_key</th>
                  <th class="text-left px-3 py-2">插件租户</th>
                  <th class="text-left px-3 py-2">pending</th>
                  <th class="text-left px-3 py-2">deferred</th>
                  <th class="text-left px-3 py-2">processing</th>
                  <th class="text-left px-3 py-2">inflight</th>
                  <th class="text-left px-3 py-2">total</th>
                  <th class="text-left px-3 py-2">history</th>
                  <th class="text-left px-3 py-2">completed</th>
                  <th class="text-left px-3 py-2">failed</th>
                  <th class="text-left px-3 py-2">last_seen_at</th>
                  <th class="text-left px-3 py-2">任务清单</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in filteredTaskQueueRows"
                  :key="`${row.tenant_key}-${row.subscriber_id}`"
                  :class="['cursor-pointer border-t border-gray-200 dark:border-gray-700', rowSeverityClass(row), queueDetailVisible && queueDetailTenantKey === row.tenant_key && queueDetailSubscriberID === row.subscriber_id ? 'ring-1 ring-primary-500/60' : '']"
                  @click="openQueueDetail(row)"
                >
                  <td class="px-3 py-2">
                    <div class="font-mono">{{ row.subscriber_id }}</div>
                    <UButton class="mt-2" size="xs" variant="soft" :loading="queueDetailLoading" @click.stop="openQueueDetail(row)">
                      任务清单
                    </UButton>
                  </td>
                  <td class="px-3 py-2">
                    <UBadge :color="row.queue_scope === 'plugin' ? 'primary' : 'neutral'" variant="soft">
                      {{ formatQueueScope(row.queue_scope) }}
                    </UBadge>
                    <div v-if="row.plugin_id" class="mt-1 font-mono text-[11px] text-gray-500 break-all">{{ row.plugin_id }}</div>
                  </td>
                  <td class="px-3 py-2">
                    <div class="font-medium text-gray-900 dark:text-white">{{ formatTenantLabel(row.tenant_key) }}</div>
                    <div v-if="formatTenantLabel(row.tenant_key) !== row.tenant_key" class="font-mono text-[11px] text-gray-500 break-all">
                      {{ row.tenant_key }}
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div class="font-mono text-[11px] break-all">{{ formatPluginTenantLabel(row) }}</div>
                  </td>
                  <td :class="['px-3 py-2', metricValueClass(row.pending, 'pending')]">
                    <button class="font-semibold hover:underline" @click.stop="openQueueDetail(row, 'pending', 'runtime')">{{ row.pending }}</button>
                  </td>
                  <td :class="['px-3 py-2', metricValueClass(row.deferred, 'deferred')]">
                    <button class="font-semibold hover:underline" @click.stop="openQueueDetail(row, 'deferred', 'runtime')">{{ row.deferred }}</button>
                  </td>
                  <td :class="['px-3 py-2', metricValueClass(row.processing, 'processing')]">
                    <button class="font-semibold hover:underline" @click.stop="openQueueDetail(row, 'processing', 'runtime')">{{ row.processing }}</button>
                  </td>
                  <td :class="['px-3 py-2', metricValueClass(row.inflight, 'inflight')]">
                    <button class="font-semibold hover:underline" @click.stop="openQueueDetail(row, 'inflight', 'runtime')">{{ row.inflight }}</button>
                  </td>
                  <td class="px-3 py-2 font-semibold">{{ rowTotalTasks(row) }}</td>
                  <td class="px-3 py-2 font-semibold">
                    <button class="hover:underline" @click.stop="openQueueDetail(row, 'pending', 'history')">{{ row.history_total ?? 0 }}</button>
                  </td>
                  <td class="px-3 py-2 text-emerald-700 dark:text-emerald-300">
                    <button class="font-semibold hover:underline" @click.stop="openQueueDetail(row, 'pending', 'history')">{{ row.completed ?? 0 }}</button>
                  </td>
                  <td :class="['px-3 py-2', Number(row.failed || 0) > 0 ? 'text-red-700 dark:text-red-300 font-semibold' : 'text-gray-500']">
                    <button class="hover:underline" @click.stop="openQueueDetail(row, 'pending', 'history')">{{ row.failed ?? 0 }}</button>
                  </td>
                  <td class="px-3 py-2 font-mono">{{ formatMonitorLogTs(row.last_seen_at) }}</td>
                  <td class="px-3 py-2">
                    <UButton size="xs" variant="outline" :loading="queueDetailLoading" @click.stop="openQueueDetail(row)">
                      查看任务
                    </UButton>
                  </td>
                </tr>
                <tr v-if="filteredTaskQueueRows.length === 0">
                  <td class="px-3 py-3 text-gray-500" colspan="14">暂无 subscriber 统计数据</td>
                </tr>
              </tbody>
            </table>
          </div>

        </UCard>

        <UModal
          v-model:open="queueDetailVisible"
          title="订阅任务清单"
          description="查看指定 subscriber + tenant_key 分片的运行态队列和历史任务。"
          :ui="{ content: 'max-w-none w-[96vw] h-[88vh]' }"
        >
          <template #body>
            <div ref="queueDetailCardRef" class="space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="space-y-1 text-sm">
                <div class="font-semibold text-gray-900 dark:text-white">订阅任务清单</div>
                <div class="text-xs text-gray-500">
                  <span>{{ formatTenantLabel(queueDetailTenantKey) }}</span>
                  <span class="mx-1">/</span>
                  <span class="font-mono">{{ queueDetailSubscriberID }}</span>
                </div>
                <div class="text-xs text-gray-500">
                  来源：{{ formatQueueScope(queueDetailScope) }}
                  <span v-if="queueDetailPluginID" class="ml-2 font-mono">{{ queueDetailPluginID }}</span>
                </div>
                <div v-if="formatTenantLabel(queueDetailTenantKey) !== queueDetailTenantKey" class="text-xs font-mono text-gray-500 break-all">
                  tenant_key: {{ queueDetailTenantKey }}
                </div>
                <div class="text-xs text-gray-500">
                  运行态 {{ queueRuntimeRows.length }} 条，历史 {{ queueDetailHistory.length }} 条
                </div>
              </div>
              <div class="flex gap-2">
                <UButton size="xs" variant="soft" @click="queueDetailVisible = false">
                  返回聚合
                </UButton>
                <UButton size="xs" variant="outline" :loading="queueDetailLoading" @click="loadQueueDetailMessages(queueDetailTenantKey, queueDetailSubscriberID, true)">
                  刷新
                </UButton>
                <UButton size="xs" variant="ghost" @click="queueDetailVisible = false">关闭</UButton>
              </div>
            </div>

            <div class="flex flex-wrap gap-2">
              <UButton size="xs" :variant="queueDetailState === 'pending' ? 'solid' : 'outline'" @click="queueDetailState = 'pending'">
                pending ({{ queueDetailMessages.pending.length }})
              </UButton>
              <UButton size="xs" :variant="queueDetailState === 'deferred' ? 'solid' : 'outline'" @click="queueDetailState = 'deferred'">
                deferred ({{ queueDetailMessages.deferred.length }})
              </UButton>
              <UButton size="xs" :variant="queueDetailState === 'processing' ? 'solid' : 'outline'" @click="queueDetailState = 'processing'">
                processing ({{ queueDetailMessages.processing.length }})
              </UButton>
              <UButton size="xs" :variant="queueDetailState === 'inflight' ? 'solid' : 'outline'" @click="queueDetailState = 'inflight'">
                inflight ({{ queueDetailMessages.inflight.length }})
              </UButton>
            </div>

            <div class="flex flex-wrap gap-2">
              <UButton size="xs" :variant="queueTaskViewMode === 'runtime' ? 'solid' : 'outline'" @click="queueTaskViewMode = 'runtime'">
                运行态队列
              </UButton>
              <UButton size="xs" :variant="queueTaskViewMode === 'history' ? 'solid' : 'outline'" @click="queueTaskViewMode = 'history'">
                任务历史 ({{ queueDetailHistory.length }})
              </UButton>
            </div>

            <div class="max-h-[56vh] overflow-auto rounded border border-gray-200 dark:border-gray-700">
              <table class="min-w-[1800px] text-xs">
                <thead class="sticky top-0 z-10 bg-gray-100 dark:bg-gray-800 shadow-sm">
                  <tr>
                    <th class="text-left px-3 py-2 w-64">task_id</th>
                    <th class="text-left px-3 py-2 w-24">来源</th>
                    <th class="text-left px-3 py-2 w-44">插件租户</th>
                    <th class="text-left px-3 py-2 w-72">topic</th>
                    <th class="text-left px-3 py-2 w-28">status</th>
                    <th class="text-left px-3 py-2 w-60">trace_id</th>
                    <th class="text-left px-3 py-2 w-20">attempt</th>
                    <th class="text-left px-3 py-2 w-24">topic_total</th>
                    <th class="text-left px-3 py-2 w-44">visible_at</th>
                    <th class="text-left px-3 py-2 w-44">submitted_at</th>
                    <th class="text-left px-3 py-2 w-44">completed_at</th>
                    <th class="text-left px-3 py-2 w-28">source</th>
                    <th class="text-left px-3 py-2 w-24">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="msg in queueUnifiedRows" :key="`${msg.source}-${msg.task_id}`" class="border-t border-gray-200 dark:border-gray-700">
                    <td class="px-3 py-2 font-mono break-all">{{ msg.task_id }}</td>
                    <td class="px-3 py-2">{{ formatQueueScope(msg.queue_scope) }}</td>
                    <td class="px-3 py-2 font-mono break-all">{{ msg.plugin_tenant_key || '-' }}</td>
                    <td class="px-3 py-2 font-mono break-all">{{ formatTopicForDisplay(msg.topic) }}</td>
                    <td class="px-3 py-2">{{ msg.status }}</td>
                    <td class="px-3 py-2 font-mono break-all">{{ msg.trace_id }}</td>
                    <td class="px-3 py-2">{{ msg.attempt }}</td>
                    <td class="px-3 py-2 font-semibold">{{ topicTaskCountMap[msg.topic] || 0 }}</td>
                    <td class="px-3 py-2 font-mono whitespace-nowrap">{{ formatMonitorLogTs(msg.visible_at) }}</td>
                    <td class="px-3 py-2 font-mono whitespace-nowrap">{{ formatMonitorLogTs(msg.submitted_at) }}</td>
                    <td class="px-3 py-2 font-mono whitespace-nowrap">{{ formatMonitorLogTs(msg.completed_at) }}</td>
                    <td class="px-3 py-2">{{ msg.source }}</td>
                    <td class="px-3 py-2">
                      <UButton size="xs" variant="outline" :loading="queueTaskDetailLoading" @click="openQueueTaskDetail(msg)">
                        详情
                      </UButton>
                    </td>
                  </tr>
                  <tr v-if="queueUnifiedRows.length === 0">
                    <td class="px-3 py-3 text-gray-500" colspan="13">当前视图没有任务。</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="queueTaskViewMode === 'history' && queueDetailPagination.total > 0" class="flex flex-wrap items-center justify-between gap-3 text-xs">
              <div class="text-gray-500">
                共 {{ queueDetailPagination.total }} 条，第 {{ queueDetailPagination.page }}/{{ queueDetailPageCount }} 页。按 last_seen_at 倒序，时间显示为 {{ monitorTimeZoneLabel }}。
              </div>
              <UPagination
                v-model:page="queueDetailPagination.page"
                :total="queueDetailPagination.total"
                :items-per-page="queueDetailPagination.pageSize"
                :sibling-count="1"
                show-edges
                @update:page="loadCurrentQueueDetailPage"
              />
            </div>
            <div class="text-xs text-gray-500">
              说明：运行态来自 Redis 队列；任务历史来自 `event_task_histories` 持久化表，默认按最近更新时间倒序。
            </div>
            </div>
          </template>
        </UModal>

        <UModal
          v-model:open="queueTaskDetailVisible"
          title="任务详情"
          description="查看任务历史记录、错误、payload 与 metadata。"
          :ui="{ content: 'max-w-5xl w-[88vw]' }"
        >
          <template #body>
            <div class="space-y-3 text-xs">
              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <div v-for="item in queueTaskDetailFields" :key="item.label" class="rounded border border-gray-200 dark:border-gray-700 p-2">
                  <div class="text-gray-500">{{ item.label }}</div>
                  <div class="mt-1 font-mono break-all">{{ item.value || '-' }}</div>
                </div>
              </div>
              <div v-if="queueTaskDetail?.error_message" class="rounded border border-red-200 bg-red-50/60 p-3 text-red-700 dark:border-red-800 dark:bg-red-950/20 dark:text-red-300">
                {{ queueTaskDetail.error_message }}
              </div>
              <div>
                <div class="mb-1 font-semibold">Payload / 请求与返回</div>
                <pre class="max-h-96 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 whitespace-pre-wrap">{{ formatTaskDetailPayload(queueTaskDetail?.payload) }}</pre>
              </div>
              <div>
                <div class="mb-1 font-semibold">Metadata</div>
                <pre class="max-h-72 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 whitespace-pre-wrap">{{ formatJson(queueTaskDetail?.metadata) }}</pre>
              </div>
            </div>
          </template>
        </UModal>

        <UCard v-if="eventSubTab === 'debug'">
          <template #header>
            <div class="font-semibold">联调面板（按钮 / 接口 / 结果联动）</div>
          </template>

          <div class="rounded border border-emerald-200 dark:border-emerald-800 bg-emerald-50/40 dark:bg-emerald-950/20 p-4 space-y-4">
            <div>
              <div class="font-semibold text-sm text-emerald-700 dark:text-emerald-300">快速开始（只做这一步）</div>
              <div class="mt-1 text-xs text-gray-600 dark:text-gray-300">
                两个联调都只发一次请求，状态依赖后端链路与 WebSocket 推送（不做前端轮询）。
              </div>
              <div class="mt-2 text-xs text-gray-600 dark:text-gray-300 space-y-1">
                <div><span class="font-semibold">Replay 联调：</span><span class="font-mono">_topic.knowledge.space.feedback.reprocess</span>，用于验证 replay/task 状态流转。</div>
                <div><span class="font-semibold">Pipeline 联调：</span><span class="font-mono">_topic.system.notification</span>，用于验证 queue 消费后通知是否到达铃铛。</div>
              </div>
              <div class="mt-2 grid grid-cols-1 md:grid-cols-2 gap-2 text-xs">
                <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                  <div class="text-gray-500">当前租户</div>
                  <div class="font-mono mt-1 break-all">{{ effectiveTenantName }}</div>
                </div>
                <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                  <div class="text-gray-500">最终发送 Topic（预览）</div>
                  <div class="font-mono mt-1 break-all">{{ replayTopicPreviewDisplay || '-' }}</div>
                </div>
              </div>
            </div>

            <div class="flex flex-wrap gap-2">
              <UButton size="sm" color="primary" :loading="flowDebug.running" :disabled="!canRunFlowDebug" @click="runFlowTemplateTask">
                Replay 联调
              </UButton>
              <UButton size="sm" color="primary" variant="solid" :loading="queueNotificationLoading" :disabled="!canRunFlowDebug" @click="runQueueNotificationDebug">
                Pipeline 联调
              </UButton>
              <UButton size="sm" variant="outline" :disabled="flowDebug.running" @click="stopFlowTemplateTask">
                清空状态
              </UButton>
            </div>
            <div v-if="!canRunFlowDebug" class="text-xs text-amber-600 dark:text-amber-400">
              当前未拿到租户上下文，已禁止发起联调请求。
            </div>

            <div class="grid grid-cols-1 gap-2 md:grid-cols-4 text-xs">
              <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                <div class="text-gray-500">当前阶段</div>
                <div class="font-semibold mt-1">{{ flowDebug.phase || '-' }}</div>
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                <div class="text-gray-500">模式</div>
                <div class="font-semibold mt-1">单次请求</div>
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                <div class="text-gray-500">任务 ID</div>
                <div class="font-mono mt-1 truncate">{{ flowDebug.taskId || '-' }}</div>
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-2">
                <div class="text-gray-500">Topic</div>
                <div class="font-mono mt-1 truncate">{{ formatTopicForDisplay(flowDebug.topic) || '-' }}</div>
              </div>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300 space-y-1">
              <div class="font-semibold">怎么判断联调成功</div>
              <div>1) Replay 联调：阶段出现 <span class="font-semibold">queued/running/completed/failed</span>。</div>
              <div>2) Pipeline 联调：Queue 页签出现消费者计数变化，且顶部通知铃铛收到新通知。</div>
            </div>

            <div class="max-h-32 overflow-auto rounded border border-emerald-200 dark:border-emerald-900 p-2 text-xs font-mono">
              <div v-for="(item, idx) in flowDebug.timeline" :key="idx" class="mb-1">
                [{{ item.ts }}] {{ item.stage }} · {{ formatTopicForDisplay(item.detail) }}
              </div>
              <div v-if="flowDebug.timeline.length === 0" class="text-gray-500">尚未开始联调。</div>
            </div>
          </div>

          <details class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-3">
            <summary class="cursor-pointer text-sm font-semibold">高级排查（按接口逐步验证）</summary>
            <div class="mt-3 space-y-3">
              <div class="text-xs text-gray-500">需要时再用：逐个接口验证，定位是总览还是队列的问题。</div>
              <div class="overflow-x-auto">
                <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
                  <thead class="bg-gray-50 dark:bg-gray-800/50">
                    <tr>
                      <th class="text-left px-3 py-2">动作</th>
                      <th class="text-left px-3 py-2">接口</th>
                      <th class="text-left px-3 py-2">影响区域</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="row in debugActionRows" :key="row.name" class="border-t border-gray-200 dark:border-gray-700">
                      <td class="px-3 py-2">{{ row.name }}</td>
                      <td class="px-3 py-2 font-mono">{{ row.endpoint }}</td>
                      <td class="px-3 py-2">{{ row.affects }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <div class="flex flex-wrap gap-2">
                <UButton size="xs" variant="outline" :loading="debugRunning" @click="runDebugOverview">校验总览</UButton>
                <UButton size="xs" variant="outline" :loading="debugRunning" @click="runDebugQueue">校验队列</UButton>
                <UButton size="xs" color="primary" :loading="debugRunning" @click="runDebugAll">一键联调</UButton>
              </div>

              <div class="max-h-32 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono">
                <div v-for="(line, idx) in debugLogs" :key="idx" class="mb-1">{{ line }}</div>
                <div v-if="debugLogs.length === 0" class="text-gray-500">点击上方按钮开始联调。</div>
              </div>
            </div>
          </details>
        </UCard>

          </div>
        </div>
      </div>

      <div v-else-if="activeTab === 'websocket'" class="space-y-4">
        <UCard>
          <template #header><div class="font-semibold">WebSocket 调试（按步骤）</div></template>
          <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300">
            <div class="font-semibold mb-2">操作顺序</div>
            <ol class="list-decimal pl-4 space-y-1">
              <li>先点「连接」。</li>
              <li>选择 topic，然后点「开始订阅」。</li>
              <li>点「触发 Replay 联调」或「触发 Pipeline 联调」。</li>
              <li>在下方「实时消息预览」确认收到消息。</li>
            </ol>
            <div class="mt-2 text-gray-500 space-y-1">
              <div>推荐订阅 topic：</div>
              <div>• 触发 Replay 联调：推荐订阅 <span class="font-mono">_topic.system.notification</span>（可看到 replay 状态事件，kind=<span class="font-mono">_kind.event_fabric.replay.task</span>）。</div>
              <div>• 触发 Pipeline 联调：推荐订阅 <span class="font-mono">_topic.system.notification</span>（可看到通知分发事件）。</div>
              <div>提示：页面不做轮询，实时变化以 WebSocket 推送为准。</div>
            </div>
          </div>

          <div class="mt-3 flex flex-wrap items-center gap-2">
            <UBadge color="neutral" variant="subtle">连接状态</UBadge>
            <UBadge :color="wsConnected ? 'success' : wsConnecting ? 'warning' : wsLastError ? 'error' : 'neutral'" variant="solid">
              {{ wsConnected ? '已连接' : wsConnecting ? '连接中' : '未连接' }}
            </UBadge>
            <UBadge color="neutral" variant="subtle">订阅状态</UBadge>
            <UBadge v-if="wsSubscribedTopic" color="success" variant="solid">{{ wsSubscribedTopic }}</UBadge>
            <UBadge v-else color="warning" variant="solid">未订阅</UBadge>
            <span class="text-xs text-gray-500" v-if="wsLastError">错误：{{ wsLastError }}</span>
          </div>

          <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-4">
            <USelectMenu
              v-model="wsTopic"
              :items="wsTopicOptions"
              value-key="value"
              label-key="label"
              placeholder="请选择订阅 topic"
              class="md:col-span-3 w-full"
            />
            <div class="text-xs text-gray-500">请选择一个 topic；若找不到，请先到事件管理页确认该 topic 已注册。</div>
          </div>

          <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
              <div class="text-xs font-semibold text-gray-500">连接控制</div>
              <div class="flex flex-wrap gap-2">
                <UButton v-if="!wsConnected && !wsConnecting" size="sm" color="primary" @click="connectWs">连接</UButton>
                <UButton v-else size="sm" color="error" variant="soft" @click="disconnectWs">断开</UButton>
              </div>
              <div class="text-xs text-gray-500">
                {{ wsConnected ? "当前已连接，可执行：断开" : wsConnecting ? "连接中，请稍候…" : "当前未连接，可执行：连接" }}
              </div>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
              <div class="text-xs font-semibold text-gray-500">订阅控制</div>
              <div class="flex flex-wrap gap-2">
                <UButton size="sm" color="primary" @click="subscribeWs">开始订阅</UButton>
                <UButton size="sm" variant="outline" @click="unsubscribeWs">停止订阅</UButton>
                <UButton size="sm" variant="ghost" @click="clearWsEvents">清空消息</UButton>
              </div>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
              <div class="text-xs font-semibold text-gray-500">联调触发</div>
              <div class="flex flex-wrap gap-2">
                <UButton size="sm" color="primary" variant="soft" :loading="flowDebug.running" :disabled="!canRunFlowDebug" @click="runFlowTemplateTask">
                  触发 Replay 联调
                </UButton>
                <UButton size="sm" color="primary" variant="soft" :loading="queueNotificationLoading" :disabled="!canRunFlowDebug" @click="runQueueNotificationDebug">
                  触发 Pipeline 联调
                </UButton>
              </div>
            </div>
          </div>
        </UCard>

        <UCard>
          <template #header><div class="font-semibold">实时消息预览（最近 20 条）</div></template>
          <div class="mb-2 text-xs text-gray-500">
            共 {{ wsEvents.length }} 条 · 最新 topic：<span class="font-mono">{{ wsEvents[0]?.topic || '-' }}</span>
          </div>
          <div class="max-h-72 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono space-y-2">
            <div v-for="(item, idx) in wsEvents" :key="idx" class="border-b border-gray-100 dark:border-gray-800 pb-2">
              <div>time: {{ item.ts }}</div>
              <div>type: {{ item.type }}</div>
              <div>topic: {{ item.topic }}</div>
              <div>trace_id: {{ item.traceId || '-' }}</div>
              <div>payload: {{ item.payload }}</div>
            </div>
            <div v-if="wsEvents.length === 0" class="text-gray-500">还没有收到实时消息。</div>
          </div>
        </UCard>
      </div>

      <div v-else-if="activeTab === 'task-cron'" class="space-y-4">
        <UCard>
          <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div class="space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-xl font-semibold text-gray-900 dark:text-white">后台任务中心</h2>
                <UBadge color="success" variant="soft">平台内置任务已接入</UBadge>
                <UBadge color="primary" variant="soft">Runtime Scheduler 已接入</UBadge>
              </div>
              <p class="max-w-3xl text-sm text-gray-600 dark:text-gray-400">
                区分平台内置后台任务和插件/运行时调度任务；队列明细与链路排查分别在事件总线和 Logs / Trace 查看。
              </p>
            </div>
            <div class="flex flex-wrap gap-2">
              <UButton size="xs" variant="outline" to="/monitor/event-bus">事件总线</UButton>
              <UButton size="xs" variant="soft" to="/monitor/logs-trace">查看 Logs / Trace</UButton>
            </div>
          </div>

          <div class="mt-4 rounded-lg border border-gray-200 bg-gray-50/80 p-3 text-xs text-gray-600 dark:border-gray-700 dark:bg-gray-900/30 dark:text-gray-300">
            平台内置后台任务来自 Event Fabric Cron；Runtime Scheduler 任务来自 <span class="font-mono">/admin/scheduler/jobs</span>，插件通过网关创建的调度会显示在 Runtime Scheduler 区域。
          </div>
        </UCard>

        <UTabs v-model="taskCronSubTab" :items="taskCronSubTabs" />

        <UCard v-if="taskCronSubTab === 'platform'">
          <template #header>
            <div class="flex items-center justify-between gap-2">
              <div>
                <div class="font-semibold">平台内置后台任务</div>
                <div class="mt-1 text-xs text-gray-500">只展示 PowerX 内置的扫描任务和队列消费任务，不包含插件创建的 Runtime Scheduler Job。</div>
              </div>
              <div class="flex flex-wrap justify-end gap-2">
                <UButton
                  size="sm"
                  variant="outline"
                  icon="i-heroicons-adjustments-horizontal"
                  @click="thresholdModalOpen = true"
                >
                  队列高亮阈值
                </UButton>
                <UButton size="sm" variant="outline" :loading="cronDebug.loading" @click="loadCronJobsDebug">刷新</UButton>
              </div>
            </div>
          </template>

          <UAlert
            icon="i-heroicons-information-circle"
            variant="subtle"
            title="当前展示的是平台内置后台任务"
            description="“重试投递扫描”是定时扫描任务；“授权超时处理”是队列消费任务。它们不是插件业务任务。"
          />

          <div class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300">
            当前时间：<span class="font-mono">{{ cronDebug.now || '-' }}</span>
          </div>

          <div class="mt-3 overflow-x-auto">
            <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
              <thead class="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th class="text-left px-3 py-2">任务名称</th>
                  <th class="text-left px-3 py-2">触发方式</th>
                  <th class="text-left px-3 py-2">运行状态</th>
                  <th class="text-left px-3 py-2">触发条件</th>
                  <th class="text-left px-3 py-2">处理内容</th>
                  <th class="text-left px-3 py-2">下次运行</th>
                  <th class="text-left px-3 py-2">最近手动触发</th>
                  <th class="text-left px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="job in cronDebug.jobs" :key="job.id" class="border-t border-gray-200 dark:border-gray-700">
                  <td class="px-3 py-2">
                    <div class="font-medium">{{ formatSchedulerJobName(job) }}</div>
                    <div class="text-gray-500 font-mono">{{ job.id }}</div>
                    <div v-if="job.description" class="mt-1 text-gray-500">{{ job.description }}</div>
                  </td>
                  <td class="px-3 py-2">{{ formatSchedulerKind(job) }}</td>
                  <td class="px-3 py-2"><UBadge :color="cronStatusColor(job.status)" variant="soft">{{ job.status }}</UBadge></td>
                  <td class="px-3 py-2">
                    {{ formatCronSchedule(job) }}
                  </td>
                  <td class="px-3 py-2 font-mono whitespace-normal break-all">
                    {{ formatSchedulerTarget(job) }}
                  </td>
                  <td class="px-3 py-2 font-mono whitespace-nowrap">{{ job.next_run_at || '-' }}</td>
                  <td class="px-3 py-2 font-mono whitespace-nowrap">{{ cronLastRunAtMap[job.id] || '-' }}</td>
                  <td class="px-3 py-2 whitespace-nowrap">
                    <div class="flex flex-nowrap gap-2 min-w-max">
                      <UButton size="xs" color="primary" :loading="cronDebug.loading" :disabled="!job.supports_run_now" @click="runCronJobNowDebug(job.id)">
                        立即执行
                      </UButton>
                      <UButton size="xs" variant="outline" :loading="cronDebug.loading" :disabled="!job.supports_pause || job.status === 'paused'" @click="pauseCronJobDebug(job.id)">
                        暂停
                      </UButton>
                      <UButton size="xs" variant="soft" color="success" :loading="cronDebug.loading" :disabled="!job.supports_pause || job.status !== 'paused'" @click="resumeCronJobDebug(job.id)">
                        恢复
                      </UButton>
                    </div>
                  </td>
                </tr>
                <tr v-if="cronDebug.jobs.length === 0">
                  <td class="px-3 py-6 text-center text-gray-500" colspan="8">
                    暂无作业数据。
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="mt-1 text-xs text-gray-500">
            这张表只代表平台 worker 定义；队列积压在事件总线查看，链路排查在 Logs / Trace 查看。
          </div>

          <div class="mt-3 flex justify-end">
            <UButton size="xs" variant="ghost" @click="cronDebug.logs = []">清空任务日志</UButton>
          </div>
          <div class="mt-2 max-h-40 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono">
            <div v-for="(line, idx) in cronDebug.logs" :key="idx" class="mb-1">{{ line }}</div>
            <div v-if="cronDebug.logs.length === 0" class="text-gray-500">暂无任务操作日志。</div>
          </div>
        </UCard>

        <UCard v-else-if="taskCronSubTab === 'runtime'">
          <template #header>
            <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div class="font-semibold">Runtime Scheduler</div>
                <div class="mt-1 text-xs text-gray-500">
                  插件或运行时通过 SchedulerService 创建的调度任务。这里显示持久化 job 和最近触发状态。
                </div>
              </div>
              <div class="flex flex-wrap justify-end gap-2">
                <UInput
                  v-model="runtimeSchedulerFilters.ownerId"
                  size="sm"
                  class="w-64"
                  placeholder="owner_id，例如 com.powerx.plugins.base"
                />
                <USelect
                  v-model="runtimeSchedulerFilters.status"
                  size="sm"
                  class="w-36"
                  :items="runtimeSchedulerStatusOptions"
                />
                <UButton size="sm" variant="outline" :loading="runtimeScheduler.loading" @click="loadRuntimeSchedulerJobs">刷新</UButton>
              </div>
            </div>
          </template>

          <UAlert
            icon="i-heroicons-clock"
            variant="subtle"
            title="当前展示的是 Runtime Scheduler Job"
            description="插件网关 Scheduler 测试创建的 once/interval/cron job 会在这里出现；once 到点触发后会变成 completed。"
          />

          <div class="mt-3 overflow-x-auto">
            <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
              <thead class="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th class="text-left px-3 py-2">任务</th>
                  <th class="text-left px-3 py-2">Owner</th>
                  <th class="text-left px-3 py-2">类型</th>
                  <th class="text-left px-3 py-2">状态</th>
                  <th class="text-left px-3 py-2">表达式</th>
                  <th class="text-left px-3 py-2">下次运行</th>
                  <th class="text-left px-3 py-2">最近触发</th>
                  <th class="text-left px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="job in runtimeScheduler.jobs" :key="runtimeSchedulerJobID(job)" class="border-t border-gray-200 dark:border-gray-700">
                  <td class="px-3 py-2">
                    <div class="font-medium">{{ job.name || '-' }}</div>
                    <div class="font-mono text-gray-500">{{ runtimeSchedulerJobID(job) }}</div>
                    <div class="mt-1 font-mono text-gray-500">{{ job.topic || '-' }}</div>
                  </td>
                  <td class="px-3 py-2">
                    <div>{{ job.owner_type || '-' }}</div>
                    <div class="font-mono text-gray-500">{{ job.owner_id || '-' }}</div>
                  </td>
                  <td class="px-3 py-2">{{ formatRuntimeSchedulerType(job) }}</td>
                  <td class="px-3 py-2">
                    <UBadge :color="runtimeSchedulerStatusColor(job.status)" variant="soft">{{ job.status || '-' }}</UBadge>
                  </td>
                  <td class="px-3 py-2 font-mono whitespace-nowrap">{{ job.schedule_expr || '-' }}</td>
                  <td class="px-3 py-2 font-mono whitespace-nowrap">{{ job.next_run_at || '-' }}</td>
                  <td class="px-3 py-2 font-mono whitespace-nowrap">{{ normalizeZeroTime(job.last_run_at) }}</td>
                  <td class="px-3 py-2 whitespace-nowrap">
                    <div class="flex flex-nowrap gap-2 min-w-max">
                      <UButton size="xs" color="primary" :loading="runtimeScheduler.loading" :disabled="!canTriggerRuntimeScheduler(job)" @click="triggerRuntimeSchedulerJob(job)">
                        立即触发
                      </UButton>
                      <UButton size="xs" variant="outline" :loading="runtimeScheduler.loading" :disabled="job.status !== 'active'" @click="pauseRuntimeSchedulerJob(job)">
                        暂停
                      </UButton>
                      <UButton size="xs" variant="soft" color="success" :loading="runtimeScheduler.loading" :disabled="job.status !== 'paused'" @click="resumeRuntimeSchedulerJob(job)">
                        恢复
                      </UButton>
                    </div>
                  </td>
                </tr>
                <tr v-if="runtimeScheduler.jobs.length === 0">
                  <td class="px-3 py-6 text-center text-gray-500" colspan="8">
                    暂无 Runtime Scheduler Job。
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
            <div class="rounded border border-gray-200 p-3 text-xs dark:border-gray-700">
              <div class="text-gray-500">总数</div>
              <div class="mt-1 text-lg font-semibold">{{ runtimeScheduler.total }}</div>
            </div>
            <div class="rounded border border-gray-200 p-3 text-xs dark:border-gray-700">
              <div class="text-gray-500">最近刷新</div>
              <div class="mt-1 font-mono">{{ runtimeScheduler.lastLoadedAt || '-' }}</div>
            </div>
            <div class="rounded border border-gray-200 p-3 text-xs dark:border-gray-700">
              <div class="text-gray-500">最近操作</div>
              <div class="mt-1 font-mono">{{ runtimeScheduler.lastAction || '-' }}</div>
            </div>
          </div>
        </UCard>

        <UModal
          v-model:open="thresholdModalOpen"
          title="队列高亮阈值"
          description="用于后台任务队列统计的 warn / danger 高亮。"
          :ui="{ content: 'max-w-2xl w-full' }"
        >
          <template #body>
            <div class="space-y-4">
              <div class="rounded border border-gray-200 p-3 text-xs text-gray-600 dark:border-gray-700 dark:text-gray-300">
                数值达到 warn 显示黄色，达到 danger 显示红色。danger 必须大于或等于 warn。
              </div>

              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div
                  v-for="metric in thresholdMetricFields"
                  :key="metric.key"
                  class="rounded border border-gray-200 p-3 dark:border-gray-700"
                >
                  <div class="mb-3 text-sm font-medium">{{ metric.label }}</div>
                  <div class="grid grid-cols-2 gap-2">
                    <UFormField label="warn">
                      <UInput v-model.number="thresholdConfig[metric.warnKey]" type="number" min="0" />
                    </UFormField>
                    <UFormField label="danger">
                      <UInput v-model.number="thresholdConfig[metric.dangerKey]" type="number" min="0" />
                    </UFormField>
                  </div>
                </div>
              </div>
            </div>
          </template>
          <template #footer>
            <div class="flex w-full items-center justify-between gap-2">
              <UButton color="neutral" variant="ghost" @click="resetThresholdConfig">重置默认</UButton>
              <div class="flex items-center gap-2">
                <UButton color="neutral" variant="soft" @click="thresholdModalOpen = false">取消</UButton>
                <UButton color="primary" @click="saveThresholdConfig">保存</UButton>
              </div>
            </div>
          </template>
        </UModal>
      </div>

      <UCard v-else>
        <template #header>
          <div class="flex items-center justify-between gap-3">
            <div class="font-semibold">Logs / Trace</div>
            <div class="flex items-center gap-2">
              <UButton size="xs" variant="outline" :loading="monitorLogsLoading" @click="refreshMonitorLogsConfig">刷新配置</UButton>
            </div>
          </div>
        </template>

        <div class="space-y-4">
          <div class="flex flex-wrap gap-2">
            <UBadge :color="monitorCapabilityColor('trace')" variant="soft">trace 查询 {{ monitorCapabilities.supports_trace_query ? '可用' : '不可用' }}</UBadge>
            <UBadge :color="monitorCapabilityColor('job')" variant="soft">job 查询 {{ monitorCapabilities.supports_job_query ? '可用' : '不可用' }}</UBadge>
            <UBadge :color="monitorCapabilityColor('policy')" variant="soft">policy 查询 {{ monitorCapabilities.supports_policy_query ? '可用' : '不可用' }}</UBadge>
            <UBadge :color="monitorCapabilityColor('grafana')" variant="soft">Grafana 深链 {{ monitorCapabilities.supports_grafana_link ? '可用' : '不可用' }}</UBadge>
          </div>

          <UAlert
            v-if="monitorCapabilityHint"
            icon="i-heroicons-information-circle"
            color="amber"
            variant="subtle"
            title="能力提示"
            :description="monitorCapabilityHint"
          />

          <UTabs v-model="logsTraceSubTab" :items="logsTraceSubTabs" />

          <div v-if="logsTraceSubTab === 'query'" class="space-y-4">
            <div class="flex flex-wrap items-center gap-2 text-xs">
              <UBadge variant="soft" color="neutral">查询驱动：{{ monitorDriverText }}</UBadge>
              <UBadge
                v-for="channel in monitorOutputChannels"
                :key="`out-${channel}`"
                variant="soft"
                color="success"
              >
                输出通道：{{ channel }}
              </UBadge>
              <UBadge v-if="monitorDriverText === 'file'" variant="soft" color="info">来源：info.log + error.log</UBadge>
              <UBadge v-if="monitorLogsQueryMeta?.degraded" variant="soft" color="warning">降级模式</UBadge>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
              <USelect
                v-model="monitorLogDriverSelection"
                :items="monitorDriverOptions"
                placeholder="日志源（auto/file/stdio/loki）"
              />
              <UInput v-model="monitorLogFilters.traceId" icon="i-heroicons-finger-print" placeholder="trace_id" :disabled="!monitorCapabilities.supports_trace_query" />
              <UInput v-model="monitorLogFilters.jobId" icon="i-heroicons-hashtag" placeholder="job_id" :disabled="!monitorCapabilities.supports_job_query" />
              <UInput v-model="monitorLogFilters.policyId" icon="i-heroicons-hashtag" placeholder="policy_id" :disabled="!monitorCapabilities.supports_policy_query" />
              <UInput v-model="monitorLogFilters.keyword" icon="i-heroicons-magnifying-glass" placeholder="关键字（message/raw）" />
              <UInput v-model="monitorLogFilters.from" placeholder="from (RFC3339)" />
              <UInput v-model="monitorLogFilters.to" placeholder="to (RFC3339)" />
            </div>

            <div class="flex flex-wrap gap-2">
              <UButton size="sm" color="primary" :loading="monitorLogsLoading" @click="queryMonitorLogs(true)">查询日志</UButton>
              <UButton size="sm" variant="outline" :disabled="!monitorGrafanaUrl" @click="openMonitorGrafana">打开 Grafana</UButton>
              <UButton size="sm" variant="ghost" @click="resetMonitorLogFilters">重置筛选</UButton>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
              <div class="flex items-center justify-between gap-2">
                <div class="text-sm font-medium">插件日志策略编排</div>
                <div class="flex items-center gap-2">
                  <UButton size="xs" variant="outline" :loading="monitorLogsLoading" @click="refreshPluginLoggingTargets">刷新插件列表</UButton>
                  <UButton size="xs" variant="soft" color="neutral" @click="togglePluginAdvancedJson">
                    {{ pluginAdvancedJsonOpen ? "收起高级 JSON" : "高级 JSON" }}
                  </UButton>
                </div>
              </div>
              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <USelectMenu
                  v-model="pluginLoggingOrch.pluginId"
                  :items="pluginLoggingTargetOptions"
                  value-key="value"
                  label-key="label"
                  placeholder="选择插件"
                />
                <div class="flex flex-wrap gap-2">
                  <UButton size="sm" variant="outline" :disabled="!pluginLoggingOrch.pluginId" :loading="monitorLogsLoading" @click="loadPluginLoggingPolicy">读取策略</UButton>
                  <UButton size="sm" color="primary" :disabled="!pluginLoggingOrch.pluginId" :loading="monitorLogsLoading" @click="savePluginLoggingPolicy">下发策略</UButton>
                  <UButton size="sm" color="warning" variant="soft" :disabled="!pluginLoggingOrch.pluginId" :loading="monitorLogsLoading" @click="probePluginLoggingPolicy">执行 Probe</UButton>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                  <div class="flex items-center justify-between gap-2">
                    <div class="text-xs text-gray-500">策略配置（表单）</div>
                    <UButton size="xs" variant="outline" :disabled="!pluginLoggingOrch.pluginId" @click="openPluginPolicyForm">编辑策略</UButton>
                  </div>
                  <div class="flex flex-wrap gap-2 text-xs">
                    <UBadge variant="soft" color="neutral">mode: {{ pluginPolicyForm.mode }}</UBadge>
                    <UBadge variant="soft" color="neutral">format: {{ pluginPolicyForm.format }}</UBadge>
                    <UBadge variant="soft" color="neutral">level: {{ pluginPolicyForm.level }}</UBadge>
                    <UBadge variant="soft" color="neutral">sinks: {{ pluginPolicyForm.sinksText || "-" }}</UBadge>
                    <UBadge variant="soft" :color="pluginPolicyForm.retryEnabled ? 'success' : 'warning'">
                      retry: {{ pluginPolicyForm.retryEnabled ? "on" : "off" }}
                    </UBadge>
                  </div>
                </div>
                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                  <div class="flex items-center justify-between gap-2">
                    <div class="text-xs text-gray-500">Probe 配置（表单）</div>
                    <UButton size="xs" variant="outline" :disabled="!pluginLoggingOrch.pluginId" @click="openPluginProbeForm">编辑 Probe</UButton>
                  </div>
                  <div class="text-xs text-gray-500 space-y-1">
                    <div><span class="font-mono">message</span>: {{ pluginProbeForm.message }}</div>
                    <div><span class="font-mono">component</span>: {{ pluginProbeForm.component }}</div>
                    <div><span class="font-mono">trace_id</span>: {{ pluginProbeForm.traceId }}</div>
                    <div><span class="font-mono">tenant_uuid</span>: {{ pluginProbeForm.tenantUuid || "-" }}</div>
                  </div>
                </div>
              </div>

              <div v-if="pluginAdvancedJsonOpen" class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div class="space-y-2">
                  <div class="text-xs text-amber-400">策略 JSON（高级模式，PUT /policy）</div>
                  <UTextarea v-model="pluginLoggingOrch.policyJson" :rows="10" class="font-mono text-xs" />
                </div>
                <div class="space-y-2">
                  <div class="text-xs text-amber-400">Probe JSON（高级模式，POST /probe）</div>
                  <UTextarea v-model="pluginLoggingOrch.probeJson" :rows="10" class="font-mono text-xs" />
                </div>
                <div class="md:col-span-2">
                  <UButton size="xs" variant="outline" @click="applyAdvancedJsonToForms">从高级 JSON 同步到表单</UButton>
                </div>
              </div>
              <div class="space-y-2">
                <div class="text-xs text-gray-500">最近结果</div>
                <pre class="rounded bg-gray-900 text-gray-100 text-xs p-3 overflow-x-auto">{{ pluginLoggingOrch.resultJson || "-" }}</pre>
              </div>
            </div>

            <UModal
              v-model:open="pluginPolicyFormOpen"
              title="插件日志策略配置"
              description="优先使用表单配置；未知字段可切换高级 JSON。"
              :ui="{ content: 'max-w-3xl w-full' }"
            >
              <template #body>
                <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <UFormField label="Mode">
                    <USelect v-model="pluginPolicyForm.mode" :items="pluginPolicyModeOptions" />
                  </UFormField>
                  <UFormField label="Format">
                    <USelect v-model="pluginPolicyForm.format" :items="pluginPolicyFormatOptions" />
                  </UFormField>
                  <UFormField label="Level">
                    <USelect v-model="pluginPolicyForm.level" :items="pluginLevelOptions" />
                  </UFormField>
                  <UFormField label="Sinks" description="逗号分隔，如 stdout,file">
                    <UInput v-model="pluginPolicyForm.sinksText" placeholder="stdout" />
                  </UFormField>
                  <UFormField label="Retry Enabled">
                    <UCheckbox v-model="pluginPolicyForm.retryEnabled" />
                  </UFormField>
                  <UFormField label="Retry Max Attempts">
                    <UInput v-model.number="pluginPolicyForm.retryMaxAttempts" type="number" min="1" />
                  </UFormField>
                  <UFormField label="Retry Backoff (ms)" class="md:col-span-2">
                    <UInput v-model.number="pluginPolicyForm.retryBackoffMs" type="number" min="0" />
                  </UFormField>
                </div>
              </template>
              <template #footer>
                <div class="flex items-center justify-end gap-2">
                  <UButton color="neutral" variant="soft" @click="pluginPolicyFormOpen = false">取消</UButton>
                  <UButton color="primary" @click="applyPluginPolicyForm">应用到策略</UButton>
                </div>
              </template>
            </UModal>

            <UModal
              v-model:open="pluginProbeFormOpen"
              title="插件 Probe 配置"
              description="用于生成探测日志事件。"
              :ui="{ content: 'max-w-3xl w-full' }"
            >
              <template #body>
                <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                  <UFormField label="Message" class="md:col-span-2">
                    <UInput v-model="pluginProbeForm.message" />
                  </UFormField>
                  <UFormField label="Level">
                    <USelect v-model="pluginProbeForm.level" :items="pluginLevelOptions" />
                  </UFormField>
                  <UFormField label="Component">
                    <UInput v-model="pluginProbeForm.component" />
                  </UFormField>
                  <UFormField label="Trace ID">
                    <UInput v-model="pluginProbeForm.traceId" />
                  </UFormField>
                  <UFormField label="Tenant UUID">
                    <UInput v-model="pluginProbeForm.tenantUuid" />
                  </UFormField>
                </div>
              </template>
              <template #footer>
                <div class="flex items-center justify-end gap-2">
                  <UButton color="neutral" variant="soft" @click="pluginProbeFormOpen = false">取消</UButton>
                  <UButton color="primary" @click="applyPluginProbeForm">应用到 Probe</UButton>
                </div>
              </template>
            </UModal>

            <div class="text-xs text-gray-500">
              共 {{ monitorLogsTotal }} 条，当前第 {{ monitorLogsPage }} 页（每页 {{ monitorLogsPageSize }} 条）
            </div>

            <div class="overflow-x-auto">
              <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
                <thead class="bg-gray-50 dark:bg-gray-800/50">
                  <tr>
                    <th class="text-left px-3 py-2">时间</th>
                    <th class="text-left px-3 py-2">级别</th>
                    <th class="text-left px-3 py-2">模块</th>
                    <th class="text-left px-3 py-2">trace_id</th>
                    <th class="text-left px-3 py-2">job_id</th>
                    <th class="text-left px-3 py-2">policy_id</th>
                    <th class="text-left px-3 py-2">消息</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, idx) in monitorLogsItems" :key="`${row.ts}-${idx}`" class="border-t border-gray-200 dark:border-gray-700 hover:bg-gray-50/40 dark:hover:bg-gray-800/20">
                    <td :class="['px-3 py-2 font-mono whitespace-nowrap', monitorTimestampClass(row.level)]">{{ formatMonitorLogTs(row.ts) }}</td>
                    <td class="px-3 py-2"><UBadge :color="monitorLevelColor(row.level)" variant="soft">{{ row.level || '-' }}</UBadge></td>
                    <td class="px-3 py-2 font-mono text-cyan-300">{{ row.module || '-' }}</td>
                    <td class="px-3 py-2 font-mono break-all">{{ row.trace_id || '-' }}</td>
                    <td class="px-3 py-2 font-mono">{{ row.job_id || '-' }}</td>
                    <td class="px-3 py-2 font-mono">{{ row.policy_id || '-' }}</td>
                    <td class="px-3 py-2 max-w-[56rem] whitespace-pre-wrap break-words leading-5 text-gray-100">{{ row.message || row.raw || '-' }}</td>
                  </tr>
                  <tr v-if="monitorLogsItems.length === 0">
                    <td class="px-3 py-3 text-gray-500" colspan="7">暂无日志数据，先执行一次查询。</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="flex items-center gap-2 text-xs text-gray-500">
                <span>每页</span>
                <USelectMenu
                  v-model="monitorLogPageSizeDraft"
                  :items="monitorLogPageSizeOptions"
                  value-key="value"
                  label-key="label"
                  class="w-24"
                />
                <UButton size="xs" variant="outline" :loading="monitorLogsLoading" @click="applyMonitorLogPageSize">应用</UButton>
              </div>
              <div class="flex items-center gap-2">
                <UButton size="xs" variant="outline" :disabled="monitorLogsLoading || !canMonitorLogPrevPage" @click="prevMonitorLogsPage">上一页</UButton>
                <span class="text-xs text-gray-500">第 {{ monitorLogsPage }} / {{ monitorLogsTotalPages }} 页</span>
                <UButton size="xs" variant="outline" :disabled="monitorLogsLoading || !canMonitorLogNextPage" @click="nextMonitorLogsPage">下一页</UButton>
              </div>
            </div>
          </div>

          <div v-else class="space-y-4">
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
              <div class="flex items-center justify-between gap-2">
                <div class="text-sm font-medium">日志清理策略（单策略）</div>
                <div class="flex items-center gap-2">
                  <UButton size="xs" variant="ghost" :loading="monitorLogsLoading" @click="refreshMonitorRetentionPolicy">刷新策略</UButton>
                  <UButton size="xs" color="primary" :loading="monitorLogsLoading" @click="saveMonitorRetentionPolicy">保存策略</UButton>
                </div>
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-3 bg-gray-50/60 dark:bg-gray-900/30">
                <div class="text-xs text-gray-600 dark:text-gray-300 space-y-1">
                  <div>当前模式：全局只保存一套日志清理策略。</div>
                  <div>作用范围：删除过期日志文件 + 清理日志表历史数据。</div>
                  <div>生效方式：点击“保存策略”后立即生效，不需要重启。</div>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 md:grid-cols-12">
                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 md:col-span-12">
                  <div class="flex items-center gap-2">
                    <UCheckbox v-model="retentionPolicyForm.enabled" />
                    <span class="text-sm font-medium">开启自动清理</span>
                  </div>
                  <div class="mt-1 text-xs text-gray-500">关闭后不会按计划自动执行，但仍可手动执行一次清理。</div>
                </div>

                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2 md:col-span-8">
                  <div class="text-xs text-gray-500">执行频率（Cron）</div>
                  <USelectMenu
                    v-model="retentionScheduleForm.mode"
                    :items="retentionScheduleModeOptions"
                    value-key="value"
                    label-key="label"
                    class="w-full"
                  />
                  <div v-if="retentionScheduleForm.mode === 'minutes'" class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">每</span>
                    <UInput v-model.number="retentionScheduleForm.every" type="number" min="1" max="59" class="w-28" />
                    <span class="text-xs text-gray-500">分钟执行一次</span>
                  </div>
                  <div v-else-if="retentionScheduleForm.mode === 'hours'" class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">每</span>
                    <UInput v-model.number="retentionScheduleForm.every" type="number" min="1" max="23" class="w-28" />
                    <span class="text-xs text-gray-500">小时执行一次</span>
                  </div>
                  <div v-else-if="retentionScheduleForm.mode === 'daily'" class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">每天</span>
                    <UInput v-model.number="retentionScheduleForm.dailyHour" type="number" min="0" max="23" class="w-24" />
                    <span class="text-xs text-gray-500">:</span>
                    <UInput v-model.number="retentionScheduleForm.dailyMinute" type="number" min="0" max="59" class="w-24" />
                    <span class="text-xs text-gray-500">执行</span>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="text-xs text-gray-500">自定义 Cron（5段：分 时 日 月 周）</div>
                    <UInput
                      v-model="retentionScheduleForm.customCron"
                      class="font-mono"
                      placeholder="例如：10 3 * * *"
                    />
                  </div>
                  <div class="text-xs text-gray-500">
                    生成结果：<span class="font-mono">{{ retentionPolicyForm.cron || "-" }}</span>
                  </div>
                </div>

                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2 md:col-span-4">
                  <div class="text-xs text-gray-500">执行时区</div>
                  <UInput v-model="retentionPolicyForm.timezone" placeholder="例如：Asia/Shanghai" />
                  <div class="text-xs text-gray-500">推荐固定为 `Asia/Shanghai`，避免跨时区误差。</div>
                </div>

                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2 md:col-span-4">
                  <div class="text-xs text-gray-500">保留时长（天）</div>
                  <UInput v-model.number="retentionPolicyForm.defaultRetentionDays" type="number" min="1" placeholder="例如：30" />
                  <div class="text-xs text-gray-500">超过该天数的日志会被清理。</div>
                </div>

                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2 md:col-span-8">
                  <div class="text-xs text-gray-500">当前生效策略</div>
                  <div class="text-xs whitespace-normal break-all">
                    <span class="font-medium">状态：</span>{{ monitorRetention.enabled ? "已启用" : "未启用" }}
                    <span class="mx-2 text-gray-300">|</span>
                    <span class="font-medium">Cron：</span><span class="font-mono">{{ monitorRetention.cron || "-" }}</span>
                    <span class="mx-2 text-gray-300">|</span>
                    <span class="font-medium">时区：</span><span class="font-mono">{{ monitorRetention.timezone || "-" }}</span>
                  </div>
                  <div class="text-xs">
                    <span class="font-medium">下一次执行：</span>
                    <span class="font-mono">{{ formatMonitorRetentionTs(monitorRetention.next_run) }}</span>
                  </div>
                </div>

                <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2 md:col-span-12">
                  <div class="text-xs text-gray-500">日志文件目录（多个用逗号分隔）</div>
                  <UInput v-model="retentionPolicyForm.filePathsText" placeholder="例如：logs,logs/audit,/var/log/powerx" />
                  <div class="text-xs text-gray-500">
                    只填目录，不填具体文件名。程序会在这些目录下按保留期删除过期文件。
                  </div>
                </div>
              </div>

              <details class="rounded border border-gray-200 dark:border-gray-700 p-3">
                <summary class="cursor-pointer text-sm font-medium">性能保护参数（一般保持默认）</summary>
                <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
                  <div class="space-y-2">
                    <div class="text-xs text-gray-500">每批删除条数</div>
                    <UInput v-model.number="retentionPolicyForm.batchSize" type="number" min="1" placeholder="例如：5000" />
                    <div class="text-xs text-gray-500">值越小越稳，但总耗时会更长。</div>
                  </div>
                  <div class="space-y-2">
                    <div class="text-xs text-gray-500">单次最多删除条数</div>
                    <UInput v-model.number="retentionPolicyForm.maxDeleteRowsPerRun" type="number" min="1" placeholder="例如：200000" />
                    <div class="text-xs text-gray-500">防止一次任务删太多导致数据库压力过大。</div>
                  </div>
                </div>
              </details>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div class="rounded border border-amber-200 dark:border-amber-800 p-3 space-y-2">
                  <div class="text-sm font-medium text-amber-500">试运行（不删除）</div>
                  <div class="text-xs text-gray-500">按下方输入的天数估算影响范围，不会删除数据。</div>
                  <div class="flex items-center gap-2">
                    <UInput v-model.number="retentionDryRunDays" type="number" min="0" class="w-40" placeholder="例如：0" />
                    <span class="text-xs text-gray-500 whitespace-nowrap">天</span>
                    <UButton size="sm" color="warning" variant="soft" :loading="monitorLogsLoading" @click="triggerMonitorRetentionDryRun">执行试运行</UButton>
                  </div>
                </div>

                <div class="rounded border border-emerald-200 dark:border-emerald-800 p-3 space-y-2">
                  <div class="text-sm font-medium text-emerald-500">真实清理（会删除）</div>
                  <div class="text-xs text-gray-500">
                    按已保存策略执行，不读取左侧试运行天数。
                    当前策略：保留 <span class="font-mono">{{ monitorRetentionPolicy.default_retention_days || 30 }}</span> 天。
                  </div>
                  <div class="flex items-center gap-2">
                    <UButton size="sm" color="primary" variant="outline" :loading="monitorLogsLoading" @click="triggerMonitorRetentionRun">按策略清理一次</UButton>
                    <UButton size="sm" variant="ghost" :loading="monitorLogsLoading" @click="refreshMonitorRetentionRuns">刷新任务列表</UButton>
                  </div>
                </div>
              </div>
              <div class="text-xs text-gray-500">建议流程：先试运行看命中范围，再按策略执行真实清理。</div>
            </div>

            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
              <div class="flex items-center justify-between gap-2">
                <div class="text-sm font-medium">
                  日志保留任务（{{ monitorRetention.enabled ? "已启用" : "未启用" }}）
                </div>
                <div class="text-xs text-gray-500">
                  cron={{ monitorRetention.cron || "-" }} / timezone={{ monitorRetention.timezone || "-" }} / next={{ formatMonitorRetentionTs(monitorRetention.next_run) }}
                </div>
              </div>
              <div class="overflow-x-auto">
                <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
                  <thead class="bg-gray-50 dark:bg-gray-800/50">
                    <tr>
                      <th class="text-left px-3 py-2">开始时间</th>
                      <th class="text-left px-3 py-2">模式</th>
                      <th class="text-left px-3 py-2">状态</th>
                      <th class="text-left px-3 py-2">触发来源</th>
                      <th class="text-left px-3 py-2">删除文件</th>
                      <th class="text-left px-3 py-2">删除记录</th>
                      <th class="text-left px-3 py-2">耗时(ms)</th>
                      <th class="text-left px-3 py-2">明细</th>
                      <th class="text-left px-3 py-2">错误摘要</th>
                    </tr>
                  </thead>
                  <tbody>
                    <template v-for="item in monitorRetention.items" :key="item.run_id">
                      <tr class="border-t border-gray-200 dark:border-gray-700">
                        <td class="px-3 py-2 font-mono whitespace-nowrap">{{ formatMonitorRetentionTs(item.started_at) }}</td>
                        <td class="px-3 py-2">
                          <UBadge :color="item.dry_run ? 'warning' : 'primary'" variant="soft">{{ item.dry_run ? "试运行" : "真实执行" }}</UBadge>
                        </td>
                        <td class="px-3 py-2"><UBadge :color="item.status === 'success' ? 'success' : 'error'" variant="soft">{{ item.status }}</UBadge></td>
                        <td class="px-3 py-2 font-mono">{{ item.triggered_by || "-" }}</td>
                        <td class="px-3 py-2 font-mono">{{ item.deleted_files ?? 0 }}</td>
                        <td class="px-3 py-2 font-mono">{{ item.deleted_rows ?? 0 }}</td>
                        <td class="px-3 py-2 font-mono">{{ item.duration_ms ?? 0 }}</td>
                        <td class="px-3 py-2">
                          <UButton
                            v-if="item.dry_run && (item.preview_details || []).length > 0"
                            size="xs"
                            variant="soft"
                            color="neutral"
                            @click="toggleRetentionRunDetails(item.run_id)"
                          >
                            {{ retentionDetailRunID === item.run_id ? "收起" : "查看" }}
                          </UButton>
                          <span v-else class="text-gray-500">-</span>
                        </td>
                        <td class="px-3 py-2 whitespace-normal break-all">{{ item.error_summary || "-" }}</td>
                      </tr>
                      <tr
                        v-if="retentionDetailRunID === item.run_id && item.dry_run && (item.preview_details || []).length > 0"
                        class="border-t border-gray-200 dark:border-gray-700"
                      >
                        <td class="px-3 py-2 text-xs text-gray-300" colspan="9">
                          <div class="space-y-1">
                            <div class="flex flex-wrap items-center justify-between gap-2 text-gray-400">
                              <span>预估明细（命中总数：{{ item.deleted_files ?? 0 }}，当前展示：{{ (item.preview_details || []).length }}，仅样本）</span>
                              <div class="flex items-center gap-2">
                                <UButton
                                  size="xs"
                                  variant="outline"
                                  :loading="Boolean(retentionExportLoadingMap[item.run_id])"
                                  @click="exportRetentionDetails(item, 'txt')"
                                >
                                  导出全量 TXT
                                </UButton>
                                <UButton
                                  size="xs"
                                  variant="outline"
                                  :loading="Boolean(retentionExportLoadingMap[item.run_id])"
                                  @click="exportRetentionDetails(item, 'json')"
                                >
                                  导出全量 JSON
                                </UButton>
                              </div>
                            </div>
                            <div
                              v-for="(line, lineIdx) in item.preview_details"
                              :key="`${item.run_id}-detail-${lineIdx}`"
                              class="font-mono break-all"
                            >
                              {{ line }}
                            </div>
                          </div>
                        </td>
                      </tr>
                    </template>
                    <tr v-if="(monitorRetention.items || []).length === 0">
                      <td class="px-3 py-3 text-gray-500" colspan="9">暂无日志保留任务记录。</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import {
  useEventFabricService,
  type EventFabricOverview,
  type EventFabricCronJob,
  type RuntimeSchedulerJob,
  type EventFabricTaskQueueStats,
  type EventFabricTaskQueueMessage,
} from "~/composables/api/services/eventFabricService";
import { useTenantService, type Tenant } from "~/composables/api/services/tenantService";
import { EVENT_NOTIFICATION_KIND, EVENT_SUBSCRIBERS, EVENT_TOPICS } from "~/composables/domain/eventTopic";
import { useWSBus } from "~/composables/useWSBus";
import { useMonitorLogsStore } from "~/stores/monitorLogs";

type MonitorTabKey = "event-fabric" | "websocket" | "task-cron" | "logs-trace";
type EventSubTabKey = "queue" | "debug";
type TaskDebugMode = "replay" | "pipeline" | "retry";
type TaskCronSubTabKey = "platform" | "runtime";
type LogsTraceSubTabKey = "query" | "retention";
type MonitorCenterProps = {
  forcedTab?: MonitorTabKey | "";
  hideTopTabs?: boolean;
};
const props = withDefaults(defineProps<MonitorCenterProps>(), {
  forcedTab: "",
  hideTopTabs: false,
});
const monitorTabs: Array<{ key: MonitorTabKey; label: string }> = [
  { key: "event-fabric", label: "事件总线" },
  { key: "websocket", label: "WebSocket" },
  { key: "task-cron", label: "后台任务" },
  { key: "logs-trace", label: "Logs / Trace" },
];
const eventSubTabs: Array<{ key: EventSubTabKey; label: string }> = [
  { key: "queue", label: "Queue" },
  { key: "debug", label: "联调" },
];
const taskModeOptions: Array<{ label: string; value: TaskDebugMode }> = [
  { label: "Replay", value: "replay" },
  { label: "Pipeline", value: "pipeline" },
  { label: "Retry", value: "retry" },
];
const taskCronSubTabs: Array<{ label: string; value: TaskCronSubTabKey }> = [
  { label: "平台内置任务", value: "platform" },
  { label: "Runtime Scheduler", value: "runtime" },
];
const logsTraceSubTabs: Array<{ label: string; value: LogsTraceSubTabKey }> = [
  { label: "日志查询", value: "query" },
  { label: "日志清理", value: "retention" },
];

const userStore = useUserStore();
const { isRoot, currentTenantUuid, currentTenant } = storeToRefs(userStore);
const allowAccess = computed(() => isRoot.value);
const effectiveTenantUuid = computed(() => {
  const fromOverview = String(overview.value?.tenant_uuid || "").trim();
  if (fromOverview) return fromOverview;
  return String(currentTenantUuid.value || "").trim();
});

const effectiveTenantName = computed(() => {
  const fromStore = String(currentTenant.value?.tenant_name || "").trim();
  if (fromStore) return fromStore;
  if (effectiveTenantUuid.value) return "当前租户";
  return "租户上下文未就绪";
});

const route = useRoute();
const router = useRouter();
const resolveTab = (value: unknown): MonitorTabKey => {
  const candidate = String(value || "").trim() as MonitorTabKey;
  return monitorTabs.some((tab) => tab.key === candidate)
    ? candidate
    : "event-fabric";
};
const resolvedForcedTab = computed<MonitorTabKey | "">(() => {
  const value = String(props.forcedTab || "").trim();
  if (!value) return "";
  return resolveTab(value);
});
const showTopTabs = computed(() => !props.hideTopTabs && !resolvedForcedTab.value);
const activeTab = ref<MonitorTabKey>(resolvedForcedTab.value || resolveTab(route.query.tab));
const eventSubTab = ref<EventSubTabKey>("queue");
const taskCronSubTab = ref<TaskCronSubTabKey>("platform");
const logsTraceSubTab = ref<LogsTraceSubTabKey>("query");
const setTab = (tab: MonitorTabKey) => {
  if (resolvedForcedTab.value) return;
  activeTab.value = tab;
};
const setEventSubTab = (tab: EventSubTabKey) => {
  eventSubTab.value = tab;
};

watch(activeTab, async (tab) => {
  if (!resolvedForcedTab.value) {
    await router.replace({ query: { ...route.query, tab } });
  }
  if (tab === "event-fabric") {
    if (!eventSubTabs.some((item) => item.key === eventSubTab.value)) {
      eventSubTab.value = "queue";
    }
    await refresh();
  }
});
watch(resolvedForcedTab, (tab) => {
  if (tab) {
    activeTab.value = tab;
  }
});

const svc = useEventFabricService();
const tenantSvc = useTenantService();
const toast = useToast();
const monitorLogsStore = useMonitorLogsStore();
const {
  config: monitorLogsConfig,
  items: monitorLogsItems,
  loading: monitorLogsLoading,
  page: monitorLogsPage,
  pageSize: monitorLogsPageSize,
  total: monitorLogsTotal,
  queryMeta: monitorLogsQueryMeta,
  loaded: monitorLogsLoaded,
  pluginTargets: monitorPluginTargets,
  retention: monitorRetention,
  retentionPolicy: monitorRetentionPolicy,
} = storeToRefs(monitorLogsStore);

const monitorLogFilters = reactive({
  traceId: "",
  jobId: "",
  policyId: "",
  keyword: "",
  from: "",
  to: "",
});
const monitorDriverOptions = ["auto", "file", "stdio", "loki"];
const monitorLogDriverSelection = ref("auto");

const monitorLogPageSizeOptions = [
  { label: "20", value: 20 },
  { label: "50", value: 50 },
  { label: "100", value: 100 },
];
const monitorLogPageSizeDraft = ref(50);

const pluginPolicyModeOptions = [
  { label: "host", value: "host" },
  { label: "plugin", value: "plugin" },
];
const pluginPolicyFormatOptions = [
  { label: "json", value: "json" },
  { label: "text", value: "text" },
];
const pluginLevelOptions = [
  { label: "debug", value: "debug" },
  { label: "info", value: "info" },
  { label: "warn", value: "warn" },
  { label: "error", value: "error" },
];

const pluginPolicyFormOpen = ref(false);
const pluginProbeFormOpen = ref(false);
const pluginAdvancedJsonOpen = ref(false);

const pluginPolicyForm = reactive({
  mode: "host",
  format: "json",
  level: "info",
  sinksText: "stdout",
  retryEnabled: true,
  retryMaxAttempts: 3,
  retryBackoffMs: 200,
});

const pluginProbeForm = reactive({
  message: "monitor plugin logger probe",
  level: "info",
  component: "monitor.logs.ui",
  traceId: "monitor-probe-001",
  tenantUuid: "",
});

const pluginLoggingOrch = reactive({
  pluginId: "",
  policyJson: "{\n  \"mode\": \"host\",\n  \"sinks\": [\"stdout\"],\n  \"format\": \"json\",\n  \"level\": \"info\",\n  \"retry\": {\n    \"enabled\": true,\n    \"max_attempts\": 3,\n    \"backoff_ms\": 200\n  }\n}",
  probeJson: "{\n  \"message\": \"monitor plugin logger probe\",\n  \"level\": \"info\",\n  \"component\": \"monitor.logs.ui\",\n  \"trace_id\": \"monitor-probe-001\",\n  \"tenant_uuid\": \"\"\n}",
  resultJson: "",
});

const retentionPolicyForm = reactive({
  enabled: false,
  cron: "",
  timezone: "",
  defaultRetentionDays: 30,
  batchSize: 5000,
  maxDeleteRowsPerRun: 200000,
  filePathsText: "",
});
const retentionDryRunDays = ref(0);
const retentionDetailRunID = ref("");
const retentionExportLoadingMap = reactive<Record<string, boolean>>({});

type RetentionScheduleMode = "minutes" | "hours" | "daily" | "custom";

const retentionScheduleModeOptions: Array<{ label: string; value: RetentionScheduleMode }> = [
  { label: "每 N 分钟", value: "minutes" },
  { label: "每 N 小时", value: "hours" },
  { label: "每天固定时间", value: "daily" },
  { label: "自定义 Cron", value: "custom" },
];

const retentionScheduleForm = reactive({
  mode: "daily" as RetentionScheduleMode,
  every: 15,
  dailyHour: 3,
  dailyMinute: 10,
  customCron: "",
});

const loading = ref(false);
const overview = ref<EventFabricOverview | null>(null);
const replayLimit = 20;

const filters = reactive({
  namespace: "",
  name: "",
  subscriberId: "",
});

function applyFilterPreset(preset: "feedback_reprocess" | "corpus_check" | "clear") {
  if (preset === "feedback_reprocess") {
    filters.namespace = "_topic.knowledge.space.feedback";
    filters.name = "reprocess";
    filters.subscriberId = EVENT_SUBSCRIBERS.KNOWLEDGE_REPROCESS;
    return;
  }
  if (preset === "corpus_check") {
    filters.namespace = "_topic.knowledge.space.corpuscheck";
    filters.name = "run";
    filters.subscriberId = EVENT_SUBSCRIBERS.KNOWLEDGE_CORPUS_CHECK;
    return;
  }
  filters.namespace = "";
  filters.name = "";
  filters.subscriberId = "";
}

const taskQueueLoading = ref(false);
const taskQueueStats = ref<EventFabricTaskQueueStats | null>(null);
const taskQueueNow = ref("");
const queueStatsDirty = ref(false);
const queueTenants = ref<Tenant[]>([]);
const selectedQueueTenant = ref("");
const queueTenantSearch = ref("");
const selectedQueueScope = ref<"all" | "powerx" | "plugin">("all");
const queueScopeOptions = [
  { label: "全部队列来源", value: "all" },
  { label: "PowerX 底座队列", value: "powerx" },
  { label: "插件队列", value: "plugin" },
];
const pluginTenantMissingValue = "__plugin_tenant_missing__";
const queueTenantAllValue = "__all_queue_tenants__";
const queueTenantPlaceholder = computed(() =>
  selectedQueueScope.value === "all"
    ? "请先选择队列来源"
    : selectedQueueScope.value === "plugin"
      ? "搜索并选择插件租户"
      : "搜索并选择 PowerX 租户"
);
const queueTenantOptions = computed(() => {
  const isPluginScope = selectedQueueScope.value === "plugin";
  const options = [{ label: isPluginScope ? "全部插件租户" : "全部 PowerX 租户", value: queueTenantAllValue }];
  const seen = new Set<string>();
  if (isPluginScope) {
    for (const row of taskQueueStats.value?.by_subscriber || []) {
      if (String(row.queue_scope || "powerx") !== "plugin") continue;
      const pluginID = String(row.plugin_id || "").trim();
      const pluginTenantKey = String((row as any).plugin_tenant_key || "").trim();
      const value = pluginTenantKey || pluginTenantMissingValue;
      if (seen.has(value)) continue;
      seen.add(value);
      const label = pluginTenantKey
        ? `${pluginTenantKey}${pluginID ? ` (${pluginID})` : ""}`
        : `未上报插件租户${pluginID ? ` (${pluginID})` : ""}`;
      options.push({ label, value });
    }
    return options;
  }
  if (selectedQueueScope.value !== "powerx") return [];
  for (const item of queueTenants.value) {
    const uuid = String(item.uuid || "").trim();
    if (!uuid || seen.has(uuid)) continue;
    seen.add(uuid);
    options.push({ label: item.name ? `${item.name} (${uuid.slice(0, 8)})` : uuid, value: uuid });
  }
  for (const row of taskQueueStats.value?.by_subscriber || []) {
    if (String(row.queue_scope || "powerx") !== "powerx") continue;
    const tenantKey = String(row.tenant_key || "").trim();
    if (!tenantKey || tenantKey === "global" || seen.has(tenantKey)) continue;
    seen.add(tenantKey);
    options.push({ label: tenantKey, value: tenantKey });
  }
  return options;
});
const queueTenantNameMap = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {};
  for (const item of queueTenants.value) {
    const uuid = String(item.uuid || "").trim();
    if (uuid) out[uuid] = String(item.name || uuid);
  }
  return out;
});
const taskQueueRows = computed(() => {
  return [...(taskQueueStats.value?.by_subscriber || [])].sort((a, b) => {
    const aHistory = Number(a.history_total || 0);
    const bHistory = Number(b.history_total || 0);
    if (aHistory !== bHistory) return bHistory - aHistory;
    const aRuntime = Number(a.pending || 0) + Number(a.deferred || 0) + Number(a.processing || 0) + Number(a.inflight || 0);
    const bRuntime = Number(b.pending || 0) + Number(b.deferred || 0) + Number(b.processing || 0) + Number(b.inflight || 0);
    if (aRuntime !== bRuntime) return bRuntime - aRuntime;
    return String(a.subscriber_id || "").localeCompare(String(b.subscriber_id || ""));
  });
});
const monitorDisplayTimeZone = "Asia/Shanghai";
const monitorTimeZoneLabel = computed(() => "北京时间");
const filteredTaskQueueRows = computed(() => {
  const subscriberNeedle = String(filters.subscriberId || "").trim().toLowerCase();
  const selectedTenantValueRaw = String(selectedQueueTenant.value || "").trim();
  const selectedTenantValue = selectedTenantValueRaw === queueTenantAllValue ? "" : selectedTenantValueRaw;
  const scopeNeedle = String(selectedQueueScope.value || "all").trim();
  return taskQueueRows.value.filter((row) => {
    if (scopeNeedle !== "all" && String(row.queue_scope || "powerx").trim() !== scopeNeedle) return false;
    if (selectedTenantValue) {
      if (scopeNeedle === "plugin") {
        const pluginTenantKey = String((row as any).plugin_tenant_key || "").trim();
        if (selectedTenantValue === pluginTenantMissingValue) {
          if (pluginTenantKey) return false;
        } else if (pluginTenantKey !== selectedTenantValue) {
          return false;
        }
      } else if (String(row.tenant_key || "").trim() !== selectedTenantValue) {
        return false;
      }
    }
    if (subscriberNeedle && !String(row.subscriber_id || "").toLowerCase().includes(subscriberNeedle)) return false;
    return true;
  });
});

watch(selectedQueueScope, () => {
  selectedQueueTenant.value = "";
  queueTenantSearch.value = "";
});

watch(queueTenantOptions, (options) => {
  const selected = String(selectedQueueTenant.value || "").trim();
  if (!selected) return;
  if (!options.some((item) => item.value === selected)) {
    selectedQueueTenant.value = "";
  }
});
const taskQueueHistorySummary = computed(() => {
  let total = 0;
  let completed = 0;
  let failed = 0;
  let lastSeenAt = "";
  for (const row of filteredTaskQueueRows.value) {
    total += Number(row.history_total || 0);
    completed += Number(row.completed || 0);
    failed += Number(row.failed || 0);
    const ts = String(row.last_seen_at || "").trim();
    if (!ts) continue;
    if (!lastSeenAt || new Date(ts).getTime() > new Date(lastSeenAt).getTime()) {
      lastSeenAt = ts;
    }
  }
  return { total, completed, failed, lastSeenAt };
});
const queueDetailLoading = ref(false);
const queueDetailVisible = ref(false);
const queueDetailCardRef = ref<any>(null);
const queueDetailTenantKey = ref("");
const queueDetailSubscriberID = ref("");
const queueDetailScope = ref("");
const queueDetailPluginID = ref("");
const queueDetailState = ref<"pending" | "deferred" | "processing" | "inflight">("pending");
const queueTaskViewMode = ref<"runtime" | "history">("runtime");
const queueDetailMessages = reactive<{
  pending: EventFabricTaskQueueMessage[];
  deferred: EventFabricTaskQueueMessage[];
  processing: EventFabricTaskQueueMessage[];
  inflight: EventFabricTaskQueueMessage[];
}>({
  pending: [],
  deferred: [],
  processing: [],
  inflight: [],
});
const queueDetailHistory = ref<Array<{
  task_id: string;
  tenant_key: string;
  subscriber_id: string;
  queue_scope?: string;
  plugin_id?: string;
  plugin_tenant_key?: string;
  topic: string;
  kind: string;
  status: string;
  trace_id?: string;
  attempt: number;
  source: string;
  submitted_at?: string;
  started_at?: string;
  completed_at?: string;
  last_seen_at?: string;
  error_message?: string;
  payload?: string;
  metadata?: Record<string, string>;
}>>([]);
const queueDetailPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
});
const queueDetailPageCount = computed(() => Math.max(1, Math.ceil(queueDetailPagination.total / queueDetailPagination.pageSize)));
const queueTaskDetailVisible = ref(false);
const queueTaskDetailLoading = ref(false);
const queueTaskDetail = ref<(typeof queueDetailHistory.value)[number] | null>(null);
const queueTaskDetailFields = computed(() => {
  const item = queueTaskDetail.value;
  return [
    { label: "task_id", value: item?.task_id || "" },
    { label: "status", value: item?.status || "" },
    { label: "topic", value: item?.topic || "" },
    { label: "subscriber_id", value: item?.subscriber_id || "" },
    { label: "tenant_key", value: item?.tenant_key || "" },
    { label: "plugin_id", value: item?.plugin_id || "" },
    { label: "plugin_tenant_key", value: item?.plugin_tenant_key || "" },
    { label: "trace_id", value: item?.trace_id || "" },
    { label: "attempt", value: String(item?.attempt ?? "") },
    { label: "submitted_at", value: item?.submitted_at || "" },
    { label: "started_at", value: item?.started_at || "" },
    { label: "completed_at", value: item?.completed_at || "" },
  ];
});
const currentQueueDetailRows = computed(() => queueDetailMessages[queueDetailState.value] || []);
const queueHistoryRows = computed(() => {
  return queueDetailHistory.value.map((item) => ({
    source: item.source || "history",
    task_id: item.task_id || "-",
    queue_scope: item.queue_scope || "powerx",
    plugin_tenant_key: item.plugin_tenant_key || "-",
    topic: item.topic || "-",
    status: item.status || "-",
    trace_id: item.trace_id || "-",
    attempt: String(item.attempt ?? 0),
    visible_at: item.last_seen_at || "-",
    submitted_at: item.submitted_at || "-",
    completed_at: item.completed_at || "-",
    raw: item,
  }));
});
const queueRuntimeRows = computed(() =>
  currentQueueDetailRows.value.map((msg) => ({
    source: "runtime",
    task_id: msg.id || "-",
    queue_scope: msg.queue_scope || "powerx",
    plugin_tenant_key: msg.plugin_tenant_key || "-",
    topic: msg.topic || "-",
    status: queueDetailState.value,
    trace_id: msg.trace_id || "-",
    attempt: String(msg.attempt ?? 0),
    visible_at: msg.visible_at || "-",
    submitted_at: "-",
    completed_at: "-",
    raw: msg,
  }))
);
const queueUnifiedRows = computed(() =>
  queueTaskViewMode.value === "runtime" ? queueRuntimeRows.value : queueHistoryRows.value
);
const topicTaskCountMap = computed<Record<string, number>>(() => {
  const map: Record<string, number> = {};
  for (const item of queueUnifiedRows.value) {
    const topic = String(item.topic || "").trim();
    if (!topic) continue;
    map[topic] = (map[topic] || 0) + 1;
  }
  return map;
});
const thresholdStorageKey = "powerx.monitor.task-queue-thresholds.v1";
const defaultThresholdConfig = {
  pendingWarn: 20,
  pendingDanger: 100,
  deferredWarn: 1,
  deferredDanger: 10,
  processingWarn: 1,
  processingDanger: 20,
  inflightWarn: 1,
  inflightDanger: 20,
} as const;
const thresholdConfig = reactive({ ...defaultThresholdConfig });
const thresholdModalOpen = ref(false);
const thresholdMetricFields: Array<{
  key: string;
  label: string;
  warnKey: keyof typeof thresholdConfig;
  dangerKey: keyof typeof thresholdConfig;
}> = [
  { key: "pending", label: "pending", warnKey: "pendingWarn", dangerKey: "pendingDanger" },
  { key: "deferred", label: "deferred", warnKey: "deferredWarn", dangerKey: "deferredDanger" },
  { key: "processing", label: "processing", warnKey: "processingWarn", dangerKey: "processingDanger" },
  { key: "inflight", label: "inflight", warnKey: "inflightWarn", dangerKey: "inflightDanger" },
];

function thresholdPairs() {
  return [
    ["pending", thresholdConfig.pendingWarn, thresholdConfig.pendingDanger],
    ["deferred", thresholdConfig.deferredWarn, thresholdConfig.deferredDanger],
    ["processing", thresholdConfig.processingWarn, thresholdConfig.processingDanger],
    ["inflight", thresholdConfig.inflightWarn, thresholdConfig.inflightDanger],
  ] as const;
}

function assignThresholdConfig(raw: Partial<typeof defaultThresholdConfig>) {
  const fallback = defaultThresholdConfig;
  thresholdConfig.pendingWarn = Number.isFinite(Number(raw.pendingWarn)) ? Math.max(0, Number(raw.pendingWarn)) : fallback.pendingWarn;
  thresholdConfig.pendingDanger = Number.isFinite(Number(raw.pendingDanger)) ? Math.max(0, Number(raw.pendingDanger)) : fallback.pendingDanger;
  thresholdConfig.deferredWarn = Number.isFinite(Number(raw.deferredWarn)) ? Math.max(0, Number(raw.deferredWarn)) : fallback.deferredWarn;
  thresholdConfig.deferredDanger = Number.isFinite(Number(raw.deferredDanger)) ? Math.max(0, Number(raw.deferredDanger)) : fallback.deferredDanger;
  thresholdConfig.processingWarn = Number.isFinite(Number(raw.processingWarn)) ? Math.max(0, Number(raw.processingWarn)) : fallback.processingWarn;
  thresholdConfig.processingDanger = Number.isFinite(Number(raw.processingDanger)) ? Math.max(0, Number(raw.processingDanger)) : fallback.processingDanger;
  thresholdConfig.inflightWarn = Number.isFinite(Number(raw.inflightWarn)) ? Math.max(0, Number(raw.inflightWarn)) : fallback.inflightWarn;
  thresholdConfig.inflightDanger = Number.isFinite(Number(raw.inflightDanger)) ? Math.max(0, Number(raw.inflightDanger)) : fallback.inflightDanger;
}

function loadThresholdConfig() {
  if (typeof window === "undefined") return;
  const raw = window.localStorage.getItem(thresholdStorageKey);
  if (!raw) {
    assignThresholdConfig(defaultThresholdConfig);
    return;
  }
  try {
    const parsed = JSON.parse(raw || "{}");
    assignThresholdConfig(parsed || {});
  } catch {
    assignThresholdConfig(defaultThresholdConfig);
  }
}

function saveThresholdConfig() {
  for (const [name, warn, danger] of thresholdPairs()) {
    if (danger < warn) {
      toast.add({ title: "阈值不合法", description: `${name}: danger 不能小于 warn`, color: "warning" });
      return;
    }
  }
  if (typeof window !== "undefined") {
    window.localStorage.setItem(thresholdStorageKey, JSON.stringify({ ...thresholdConfig }));
  }
  thresholdModalOpen.value = false;
  toast.add({ title: "阈值已保存", color: "success" });
}

function resetThresholdConfig() {
  assignThresholdConfig(defaultThresholdConfig);
  if (typeof window !== "undefined") {
    window.localStorage.removeItem(thresholdStorageKey);
  }
  toast.add({ title: "已恢复默认阈值", color: "info" });
}

async function loadQueueTenants() {
  try {
    const res = await tenantSvc.getTenants({ page: 1, page_size: 100 });
    queueTenants.value = res.data?.items || [];
  } catch (e: any) {
    queueTenants.value = [];
  }
}

function formatTenantLabel(tenantKey: string) {
  const key = String(tenantKey || "").trim();
  if (!key) return "-";
  if (key === "global") return "Global";
  return queueTenantNameMap.value[key] || key;
}

function formatPluginTenantLabel(row: { queue_scope?: string; plugin_tenant_key?: string }) {
  if (String(row.queue_scope || "powerx") !== "plugin") return "-";
  const key = String((row as any).plugin_tenant_key || "").trim();
  return key || "未上报插件租户";
}

function formatQueueScope(scope?: string) {
  return String(scope || "powerx") === "plugin" ? "插件队列" : "PowerX 底座";
}

function formatJson(value: unknown) {
  if (!value) return "-";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function formatTaskDetailPayload(value?: string) {
  const raw = String(value || "").trim();
  if (!raw) return "-";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

async function scrollToQueueDetail() {
  await nextTick();
  const raw = queueDetailCardRef.value;
  const el = raw?.$el || raw;
  if (el?.scrollIntoView) {
    el.scrollIntoView({ behavior: "smooth", block: "start" });
  }
}

async function loadTaskQueueStats(showToast = false) {
  taskQueueLoading.value = true;
  try {
    const res = await svc.getTaskQueueStats();
    taskQueueStats.value = res.data.task_queue;
    taskQueueNow.value = res.data.now || "";
    queueStatsDirty.value = false;
  } catch (e: any) {
    if (showToast) {
      toast.add({ title: "加载队列统计失败", description: e?.message || "未知错误", color: "error" });
    }
  } finally {
    taskQueueLoading.value = false;
  }
}

async function syncQueueStatsIfDirty() {
  if (!queueStatsDirty.value) return;
  if (activeTab.value !== "event-fabric" || eventSubTab.value !== "queue") return;
  await loadTaskQueueStats(false);
}

async function loadQueueDetailMessages(tenantKey: string, subscriberID: string, showToast = false) {
  if (!tenantKey || !subscriberID) return;
  queueDetailLoading.value = true;
  try {
    const res = await svc.getTaskQueueMessages({
      tenant_key: tenantKey,
      subscriber_id: subscriberID,
      limit: 30,
      page: queueDetailPagination.page,
      page_size: queueDetailPagination.pageSize,
    });
    const payload = res.data?.messages;
    queueDetailMessages.pending = payload?.pending || [];
    queueDetailMessages.deferred = payload?.deferred || [];
    queueDetailMessages.processing = payload?.processing || [];
    queueDetailMessages.inflight = payload?.inflight || [];
    queueDetailHistory.value = res.data?.history || [];
    queueDetailPagination.total = Number(res.data?.pagination?.total || queueDetailHistory.value.length || 0);
    queueDetailPagination.page = Number(res.data?.pagination?.page || queueDetailPagination.page || 1);
    queueDetailPagination.pageSize = Number(res.data?.pagination?.page_size || queueDetailPagination.pageSize || 20);
  } catch (e: any) {
    if (showToast) {
      toast.add({ title: "加载任务清单失败", description: e?.message || "未知错误", color: "error" });
    }
  } finally {
    queueDetailLoading.value = false;
  }
}

async function loadCurrentQueueDetailPage() {
  await loadQueueDetailMessages(queueDetailTenantKey.value, queueDetailSubscriberID.value, true);
}

async function openQueueTaskDetail(row: any) {
  const taskID = String(row?.task_id || "").trim();
  if (!taskID || taskID === "-") return;
  if (String(row?.source || "") === "runtime") {
    const raw = row?.raw || {};
    queueTaskDetail.value = {
      task_id: taskID,
      tenant_key: queueDetailTenantKey.value,
      subscriber_id: queueDetailSubscriberID.value,
      queue_scope: row?.queue_scope || queueDetailScope.value || "powerx",
      plugin_id: queueDetailPluginID.value,
      plugin_tenant_key: row?.plugin_tenant_key === "-" ? "" : row?.plugin_tenant_key,
      topic: row?.topic || raw?.topic || "",
      kind: "runtime_queue_message",
      status: row?.status || queueDetailState.value,
      trace_id: row?.trace_id === "-" ? "" : row?.trace_id,
      attempt: Number(row?.attempt || raw?.attempt || 0),
      source: "runtime",
      last_seen_at: row?.visible_at === "-" ? "" : row?.visible_at,
      payload: JSON.stringify(raw || {}, null, 2),
      metadata: raw?.metadata || {},
    } as any;
    queueTaskDetailVisible.value = true;
    return;
  }
  queueTaskDetailLoading.value = true;
  try {
    const res = await svc.getTaskQueueTask(taskID, {
      tenant_key: queueDetailTenantKey.value,
      subscriber_id: queueDetailSubscriberID.value,
    });
    queueTaskDetail.value = res.data as any;
    queueTaskDetailVisible.value = true;
  } catch (e: any) {
    toast.add({ title: "加载任务详情失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    queueTaskDetailLoading.value = false;
  }
}

async function openQueueDetail(
  row: { tenant_key: string; subscriber_id: string; queue_scope?: string; plugin_id?: string },
  state: "pending" | "deferred" | "processing" | "inflight" = "pending",
  viewMode: "runtime" | "history" = "history",
) {
  queueDetailTenantKey.value = String(row.tenant_key || "").trim();
  queueDetailSubscriberID.value = String(row.subscriber_id || "").trim();
  queueDetailScope.value = String(row.queue_scope || "powerx").trim();
  queueDetailPluginID.value = String(row.plugin_id || "").trim();
  queueDetailState.value = state;
  queueTaskViewMode.value = viewMode;
  queueDetailPagination.page = 1;
  queueDetailVisible.value = true;
  await loadQueueDetailMessages(queueDetailTenantKey.value, queueDetailSubscriberID.value, true);
  await scrollToQueueDetail();
}

function metricLevel(value: number, kind: "pending" | "deferred" | "processing" | "inflight") {
  const table = {
    pending: { warn: thresholdConfig.pendingWarn, danger: thresholdConfig.pendingDanger },
    deferred: { warn: thresholdConfig.deferredWarn, danger: thresholdConfig.deferredDanger },
    processing: { warn: thresholdConfig.processingWarn, danger: thresholdConfig.processingDanger },
    inflight: { warn: thresholdConfig.inflightWarn, danger: thresholdConfig.inflightDanger },
  } as const;
  const cfg = table[kind];
  if (value >= cfg.danger) return "danger";
  if (value >= cfg.warn) return "warn";
  return "ok";
}

function metricCardClass(value: number, kind: "pending" | "deferred" | "processing" | "inflight") {
  const base = "rounded border p-3";
  const level = metricLevel(value, kind);
  if (level === "danger") return `${base} border-red-300 bg-red-50/70 dark:border-red-700 dark:bg-red-950/30`;
  if (level === "warn") return `${base} border-amber-300 bg-amber-50/70 dark:border-amber-700 dark:bg-amber-950/30`;
  return `${base} border-gray-200 dark:border-gray-700`;
}

function metricValueClass(value: number, kind: "pending" | "deferred" | "processing" | "inflight") {
  const base = "text-lg font-semibold";
  const level = metricLevel(value, kind);
  if (level === "danger") return `${base} text-red-600 dark:text-red-400`;
  if (level === "warn") return `${base} text-amber-600 dark:text-amber-400`;
  return `${base} text-gray-900 dark:text-gray-100`;
}

function rowSeverityClass(row: { pending: number; deferred: number; processing: number; inflight: number }) {
  if (metricLevel(row.deferred, "deferred") === "danger" || metricLevel(row.inflight, "inflight") === "danger" || metricLevel(row.processing, "processing") === "danger" || metricLevel(row.pending, "pending") === "danger") {
    return "bg-red-50/50 dark:bg-red-950/20";
  }
  if (metricLevel(row.deferred, "deferred") === "warn" || metricLevel(row.inflight, "inflight") === "warn" || metricLevel(row.processing, "processing") === "warn" || metricLevel(row.pending, "pending") === "warn") {
    return "bg-amber-50/40 dark:bg-amber-950/20";
  }
  return "";
}

function rowTotalTasks(row: { pending: number; deferred: number; processing: number; inflight: number; total_tasks?: number }) {
  const persisted = Number(row.total_tasks ?? 0);
  if (Number.isFinite(persisted) && persisted > 0) return persisted;
  return Number(row.pending || 0) + Number(row.deferred || 0) + Number(row.processing || 0) + Number(row.inflight || 0);
}

async function refresh() {
  if (!allowAccess.value) return;
  loading.value = true;
  try {
    const res = await svc.getOverview({
      namespace: filters.namespace || undefined,
      name: filters.name || undefined,
      subscriber_id: filters.subscriberId || undefined,
      limit: replayLimit,
    });
    overview.value = res.data;
    await loadTaskQueueStats(false);
  } catch (e: any) {
    toast.add({ title: "加载失败", description: e?.message || "无法获取 overview", color: "error" });
  } finally {
    loading.value = false;
  }
}

const debugRunning = ref(false);
const debugLogs = ref<string[]>([]);
const flowDebug = reactive<{
  running: boolean;
  phase: string;
  taskId: string;
  topic: string;
  timeline: Array<{ ts: string; stage: string; detail: string }>;
}>({
  running: false,
  phase: "idle",
  taskId: "",
  topic: "",
  timeline: [],
});
const queueNotificationLoading = ref(false);

function pushFlow(stage: string, detail: string) {
  flowDebug.timeline.unshift({
    ts: new Date().toLocaleTimeString(),
    stage,
    detail,
  });
  flowDebug.timeline = flowDebug.timeline.slice(0, 80);
}

function stopFlowTemplateTask(opts?: { silent?: boolean; phase?: string }) {
  flowDebug.running = false;
  flowDebug.phase = opts?.phase || "stopped";
  if (!opts?.silent) {
    pushFlow(flowDebug.phase, "联调状态已清空");
  }
}

function resolveReplayTopic(input: string) {
  const trimmed = (input || "").trim();
  if (!trimmed) return "";

  const parts = trimmed.split(".").map((part) => part.trim()).filter(Boolean);
  if (parts.length < 2) {
    return trimmed;
  }

  const first = parts[0] || "";
  const firstLooksTenant = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(first);
  const firstLooksSystemScope = first === "global" || first === "system";

  if ((firstLooksTenant || firstLooksSystemScope) && parts.length >= 3) {
    return parts.slice(1).join(".");
  }

  return trimmed;
}

async function runFlowTemplateTask() {
  if (!(await ensureTenantContextLoaded())) {
    toast.add({ title: "缺少租户上下文", description: "无法识别当前登录租户，请重新登录后重试", color: "warning" });
    return;
  }
  if (flowDebug.running) {
    toast.add({ title: "联调进行中", description: "请先停止当前联调任务", color: "warning" });
    return;
  }

  flowDebug.running = true;
  flowDebug.phase = "creating";
  flowDebug.timeline = [];

  const replayTopicInput = taskDebug.mode === "replay"
    ? taskDebug.topic
    : EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS;
  const topicInput = (replayTopicInput || EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS).trim();
  const topic = resolveReplayTopic(topicInput);
  if (!topic) {
    flowDebug.phase = "error";
    flowDebug.running = false;
    toast.add({ title: "topic 为空", description: "请先输入联调 topic", color: "warning" });
    return;
  }
  flowDebug.topic = topic;
  taskDebug.topic = topic;
  pushFlow("create", `准备创建任务 topic=${topic}`);

  try {
    const createRes = await svc.createReplayTask({
      topic,
      trace_id: taskDebug.traceId.trim() || undefined,
      reason: "flow-debug template task",
      shadow: true,
    });
    const task = createRes.data;
    const taskId = task.id || "";
    flowDebug.taskId = taskId;
    taskDebug.taskId = taskId;
    taskDebug.status = task.status || "";
    taskDebug.resultCount = task.result_count ?? null;
    taskDebug.failureReason = task.failure_reason || "";
    flowDebug.phase = "queued";
    pushFlow("queued", `任务已创建 task_id=${taskId} status=${task.status || '-'}`);

    await refresh();
    const status = String(task.status || "").toLowerCase();
    if (status === "completed" || status === "done") {
      flowDebug.phase = "completed";
      pushFlow("completed", `任务完成 status=${status} result=${task.result_count ?? 0}`);
    } else if (status === "failed") {
      flowDebug.phase = "failed";
      pushFlow("failed", `任务失败 status=${status} reason=${task.failure_reason || "-"}`);
    } else if (status === "running") {
      flowDebug.phase = "running";
      pushFlow("running", `任务执行中 status=${status}`);
    } else {
      flowDebug.phase = "queued";
    }
    flowDebug.running = false;
  } catch (e: any) {
    flowDebug.phase = "error";
    pushFlow("error", e?.message || "创建联调模板任务失败");
    stopFlowTemplateTask();
    toast.add({ title: "联调任务创建失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function runQueueNotificationDebug() {
  if (!(await ensureTenantContextLoaded())) {
    toast.add({ title: "缺少租户上下文", description: "无法识别当前登录租户，请重新登录后重试", color: "warning" });
    return;
  }
  if (queueNotificationLoading.value) {
    return;
  }
  queueNotificationLoading.value = true;
  try {
    const res = await svc.createPipelineTask({
      title: "Pipeline 联调通知",
      content: "这条通知通过 Task 队列分发到 WS。",
      type: "system",
      category: "system",
      metadata: {
        source: "monitor.queue-debug",
      },
    });
    const taskID = String(res.data?.task_id || "").trim();
    pushFlow("queue", `通知任务已入队 task_id=${taskID || "-"}`);
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
    toast.add({
      title: "通知任务已入队",
      description: taskID || "等待消费者分发",
      color: "success",
    });
  } catch (e: any) {
    pushFlow("queue-error", e?.message || "通知联调任务入队失败");
    toast.add({ title: "通知联调失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    queueNotificationLoading.value = false;
  }
}


const debugActionRows = [
  {
    name: "校验总览",
    endpoint: "GET /admin/event-fabric/overview",
    affects: "租户信息、过滤条件、基础统计",
  },
  {
    name: "校验队列",
    endpoint: "GET /admin/event-fabric/task-queue/stats",
    affects: "Queue 运行态统计",
  },
  {
    name: "一键联调",
    endpoint: "按顺序执行 overview -> task-queue/stats",
    affects: "总览 + Queue + 调试日志",
  },
] as const;
const pushDebug = (msg: string) => {
  debugLogs.value.unshift(`[${new Date().toLocaleTimeString()}] ${msg}`);
  debugLogs.value = debugLogs.value.slice(0, 60);
};

async function runDebugOverview() {
  debugRunning.value = true;
  try {
    await refresh();
    pushDebug(`overview ok: endpoint=/admin/event-fabric/overview, pending=${taskQueueStats.value?.pending ?? 0}, deferred=${taskQueueStats.value?.deferred ?? 0}`);
  } catch (e: any) {
    pushDebug(`overview failed: ${e?.message || e}`);
  } finally {
    debugRunning.value = false;
  }
}

async function runDebugQueue() {
  debugRunning.value = true;
  try {
    await loadTaskQueueStats(false);
    pushDebug(`queue ok: endpoint=/admin/event-fabric/task-queue/stats, pending=${taskQueueStats.value?.pending ?? 0}, inflight=${taskQueueStats.value?.inflight ?? 0}`);
  } catch (e: any) {
    pushDebug(`queue failed: ${e?.message || e}`);
  } finally {
    debugRunning.value = false;
  }
}

async function runDebugAll() {
  debugLogs.value = [];
  await runDebugOverview();
  await runDebugQueue();
  pushDebug("debug done: 已按顺序执行 overview -> task-queue/stats");
}

const wsBus = useWSBus();
const wsConnected = computed(() => wsBus.connected.value);
const wsConnecting = computed(() => wsBus.connecting.value);
const wsLastError = computed(() => wsBus.lastError.value);
const wsTopic = ref(EVENT_TOPICS.SYSTEM_NOTIFICATION);
const wsTopicCatalog = ref<string[]>([]);
const wsSubscribedTopic = ref("");
const wsEvents = ref<Array<{ ts: string; type: string; topic: string; traceId: string; payload: string }>>([]);
let wsDispose: (() => void) | null = null;
let replayStatusDispose: (() => void) | null = null;
const replayStatusTopic = EVENT_TOPICS.SYSTEM_NOTIFICATION;

function applyReplayStatusEvent(payload: any) {
  const kind = String(payload?.kind || "").trim();
  if (kind !== EVENT_NOTIFICATION_KIND.EVENT_FABRIC_REPLAY_TASK) return;
  const data = payload?.data || {};
  const taskId = String(data?.task_id || "").trim();
  if (!taskId) return;
  if (!flowDebug.taskId || flowDebug.taskId !== taskId) return;

  const status = String(data?.status || "").trim().toLowerCase();
  const topic = String(data?.topic || "").trim();
  const resultCount = Number(data?.result_count ?? 0);
  const failureReason = String(data?.failure_reason || "").trim();

  if (topic) {
    flowDebug.topic = topic;
    taskDebug.topic = topic;
  }
  taskDebug.status = status;
  if (Number.isFinite(resultCount)) {
    taskDebug.resultCount = resultCount;
  }
  taskDebug.failureReason = failureReason;

  if (status === "completed" || status === "done") {
    flowDebug.phase = "completed";
    pushFlow("completed", `任务完成 status=${status} result=${resultCount}`);
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
  } else if (status === "failed") {
    flowDebug.phase = "failed";
    pushFlow("failed", `任务失败 status=${status} reason=${failureReason || "-"}`);
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
  } else if (status === "cancelled") {
    flowDebug.phase = "cancelled";
    pushFlow("cancelled", `任务取消 status=${status}`);
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
  } else if (status === "running") {
    flowDebug.phase = "running";
    pushFlow("running", `任务执行中 status=${status}`);
  } else if (status === "pending") {
    flowDebug.phase = "queued";
    pushFlow("queued", `任务排队 status=${status}`);
  }
}

function ensureReplayStatusSubscription() {
  if (replayStatusDispose) return;
  replayStatusDispose = wsBus.subscribe(replayStatusTopic, (payload) => {
    applyReplayStatusEvent(payload);
  });
}

function connectWs() {
  if (wsConnected.value) {
    toast.add({ title: "已连接", description: "当前 WebSocket 已连接", color: "info" });
    return;
  }
  wsBus.connect();
  toast.add({ title: "已发起连接", color: "info" });
}

function disconnectWs() {
  if (!wsConnected.value && !wsConnecting.value) {
    toast.add({ title: "当前未连接", color: "info" });
    return;
  }
  if (wsDispose) {
    wsDispose();
    wsDispose = null;
  }
  wsSubscribedTopic.value = "";
  wsBus.disconnect();
  toast.add({ title: "连接已断开", color: "info" });
}

function normalizeWsTopic(topic: string) {
  return resolveReplayTopic(String(topic || "").trim());
}

function clearWsEvents() {
  wsEvents.value = [];
}

const wsTopicOptions = computed<Array<{ label: string; value: string }>>(() => {
  const defaults = [
    EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS,
    EVENT_TOPICS.SYSTEM_NOTIFICATION,
  ];
  const dynamicFromOverview = (overview.value?.topics || [])
    .map((item) => normalizeWsTopic(item.full_topic || `${item.namespace}.${item.name}`))
    .filter(Boolean);
  const merged = Array.from(new Set([...defaults, ...wsTopicCatalog.value, ...dynamicFromOverview]));
  return merged.map((topic) => ({
    label: topic,
    value: topic,
  }));
});

async function loadWsTopicCatalog() {
  try {
    const res = await svc.listTopics({ page: 1, page_size: 500 });
    const items = Array.isArray(res?.data?.items) ? res.data.items : [];
    const topics = items
      .map((item: any) => normalizeWsTopic(item?.full_topic || `${item?.namespace || ""}.${item?.name || ""}`))
      .filter((item: string) => Boolean(item));
    wsTopicCatalog.value = Array.from(new Set(topics));
  } catch {
    wsTopicCatalog.value = [];
  }
}

function subscribeWs() {
  const topic = wsTopic.value.trim();
  if (!topic) {
    toast.add({ title: "请输入 topic", color: "warning" });
    return;
  }
  if (wsDispose) wsDispose();
  wsDispose = wsBus.subscribe(topic, (payload, env) => {
    wsEvents.value.unshift({
      ts: new Date(env.ts || Date.now()).toLocaleTimeString(),
      type: String(env.type || "event"),
      topic: env.topic || topic,
      traceId: String((env as any).trace_id || ""),
      payload: typeof payload === "string" ? payload : JSON.stringify(payload),
    });
    wsEvents.value = wsEvents.value.slice(0, 20);
  });
  wsSubscribedTopic.value = topic;
  toast.add({ title: "已订阅", description: topic, color: "success" });
}

function unsubscribeWs() {
  if (wsDispose) {
    wsDispose();
    wsDispose = null;
    wsSubscribedTopic.value = "";
    toast.add({ title: "已取消订阅", color: "info" });
  }
}

const taskDebug = reactive<{
  mode: TaskDebugMode;
  topic: string;
  traceId: string;
  taskId: string;
  status: string;
  resultCount: number | null;
  failureReason: string;
  queueTenantKey: string;
  queueSubscriberId: string;
  queueHitState: string;
  queueHitSource: string;
  loading: boolean;
  logs: string[];
}>({
  mode: "replay",
  topic: EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS,
  traceId: "",
  taskId: "",
  status: "",
  resultCount: null,
  failureReason: "",
  queueTenantKey: "",
  queueSubscriberId: "",
  queueHitState: "",
  queueHitSource: "",
  loading: false,
  logs: [],
});

const replayTopicPreview = computed(() => {
  const raw = (taskDebug.topic || EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS).trim();
  return resolveReplayTopic(raw);
});

const replayTopicPreviewDisplay = computed(() => {
  const topic = replayTopicPreview.value;
  const tenantUUID = effectiveTenantUuid.value;
  const tenantName = effectiveTenantName.value;
  if (!topic) return "-";
  if (tenantUUID && topic.startsWith(`${tenantUUID}.`)) {
    return `${tenantName}.${topic.slice(tenantUUID.length + 1)}`;
  }
  return topic;
});

function formatTopicForDisplay(input: string) {
  const text = String(input || "");
  const tenantUUID = effectiveTenantUuid.value;
  const tenantName = effectiveTenantName.value;
  if (!tenantUUID || !text) return text;
  return text.replaceAll(`${tenantUUID}.`, `${tenantName}.`);
}

const canRunFlowDebug = computed(() => {
  return Boolean(effectiveTenantUuid.value);
});

const monitorCapabilities = computed(() => {
  return monitorLogsConfig.value?.capabilities || {
    supports_label_query: false,
    supports_trace_query: false,
    supports_job_query: false,
    supports_policy_query: false,
    supports_grafana_link: false,
    history_limited: true,
    limitation_note: "日志能力尚未初始化",
  };
});

const monitorDriverText = computed(() => monitorLogsConfig.value?.driver || "stdio");
const monitorOutputChannels = computed(() => {
  const channels = Array.isArray(monitorLogsConfig.value?.output_channels)
    ? monitorLogsConfig.value?.output_channels || []
    : [];
  if (channels.length > 0) return channels;
  return [monitorDriverText.value];
});
const monitorCapabilityHint = computed(() => {
  const fromConfig = String(monitorCapabilities.value?.limitation_note || "").trim();
  if (fromConfig) return fromConfig;
  const fromQuery = String(monitorLogsQueryMeta.value?.hint || "").trim();
  return fromQuery;
});
const monitorGrafanaUrl = computed(() => {
  return String(monitorLogsQueryMeta.value?.grafana_url || "").trim();
});
const pluginLoggingTargetOptions = computed(() => {
  return (monitorPluginTargets.value || []).map((item) => ({
    value: item.plugin_id,
    label: `${item.name || item.plugin_id} (${item.plugin_id})`,
  }));
});

function monitorCapabilityColor(kind: "trace" | "job" | "policy" | "grafana") {
  if (kind === "trace") return monitorCapabilities.value.supports_trace_query ? "success" : "warning";
  if (kind === "job") return monitorCapabilities.value.supports_job_query ? "success" : "warning";
  if (kind === "policy") return monitorCapabilities.value.supports_policy_query ? "success" : "warning";
  return monitorCapabilities.value.supports_grafana_link ? "success" : "warning";
}

function monitorLevelColor(level: string) {
  const normalized = String(level || "").trim().toLowerCase();
  if (normalized === "error") return "error";
  if (normalized === "warn" || normalized === "warning") return "warning";
  if (normalized === "debug") return "neutral";
  return "success";
}

function monitorTimestampClass(level: string) {
  const normalized = String(level || "").trim().toLowerCase();
  if (normalized === "error") return "text-red-300";
  if (normalized === "warn" || normalized === "warning") return "text-amber-300";
  if (normalized === "debug") return "text-gray-400";
  return "text-emerald-300";
}

function formatMonitorLogTs(input?: string) {
  const raw = String(input || "").trim();
  if (!raw || raw === "-") return "-";
  const t = new Date(raw);
  if (Number.isNaN(t.getTime())) return raw;
  const parts = new Intl.DateTimeFormat("zh-CN", {
    timeZone: monitorDisplayTimeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).formatToParts(t);
  const get = (type: string) => parts.find((part) => part.type === type)?.value || "";
  return `${get("year")}-${get("month")}-${get("day")} ${get("hour")}:${get("minute")}:${get("second")}`;
}

function formatMonitorRetentionTs(input?: string) {
  const raw = String(input || "").trim();
  if (!raw) return "-";
  const t = new Date(raw);
  if (Number.isNaN(t.getTime())) return raw;
  return t.toLocaleString();
}

function resetMonitorLogFilters() {
  monitorLogDriverSelection.value = "auto";
  monitorLogFilters.traceId = "";
  monitorLogFilters.jobId = "";
  monitorLogFilters.policyId = "";
  monitorLogFilters.keyword = "";
  monitorLogFilters.from = "";
  monitorLogFilters.to = "";
}

async function refreshMonitorLogsConfig() {
  try {
    await monitorLogsStore.fetchConfig();
  } catch (e: any) {
    toast.add({ title: "刷新日志配置失败", description: e?.message || "未知错误", color: "error" });
  }
}

function parseOrchJSON(raw: string, label: string) {
  const text = String(raw || "").trim();
  if (!text) return {};
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error(`${label} 必须是 JSON 对象`);
    }
    return parsed as Record<string, any>;
  } catch (e: any) {
    throw new Error(`${label} 解析失败: ${e?.message || "invalid json"}`);
  }
}

function normalizeStringArray(raw: any): string[] {
  if (!Array.isArray(raw)) return [];
  return raw
    .map((item) => String(item || "").trim())
    .filter((item) => Boolean(item));
}

function prettyJSON(value: Record<string, any>) {
  return JSON.stringify(value || {}, null, 2);
}

function toPositiveInt(raw: any, fallback: number) {
  const v = Number(raw);
  if (!Number.isFinite(v) || v <= 0) return fallback;
  return Math.floor(v);
}

function toNonNegativeInt(raw: any, fallback: number) {
  const v = Number(raw);
  if (!Number.isFinite(v) || v < 0) return fallback;
  return Math.floor(v);
}

function buildPolicyPayloadFromForm() {
  const sinks = String(pluginPolicyForm.sinksText || "")
    .split(",")
    .map((item) => item.trim())
    .filter((item) => Boolean(item));
  return {
    mode: String(pluginPolicyForm.mode || "host"),
    sinks: sinks.length > 0 ? sinks : ["stdout"],
    format: String(pluginPolicyForm.format || "json"),
    level: String(pluginPolicyForm.level || "info"),
    retry: {
      enabled: Boolean(pluginPolicyForm.retryEnabled),
      max_attempts: toPositiveInt(pluginPolicyForm.retryMaxAttempts, 3),
      backoff_ms: toNonNegativeInt(pluginPolicyForm.retryBackoffMs, 200),
    },
  };
}

function applyPolicyFormFromPayload(payload: Record<string, any>) {
  pluginPolicyForm.mode = String(payload?.mode || "host");
  pluginPolicyForm.format = String(payload?.format || "json");
  pluginPolicyForm.level = String(payload?.level || "info");
  const sinks = normalizeStringArray(payload?.sinks);
  pluginPolicyForm.sinksText = sinks.length > 0 ? sinks.join(",") : "stdout";
  const retry = payload?.retry || {};
  pluginPolicyForm.retryEnabled = Boolean(retry?.enabled ?? true);
  pluginPolicyForm.retryMaxAttempts = toPositiveInt(retry?.max_attempts, 3);
  pluginPolicyForm.retryBackoffMs = toNonNegativeInt(retry?.backoff_ms, 200);
}

function buildProbePayloadFromForm() {
  return {
    message: String(pluginProbeForm.message || "monitor plugin logger probe"),
    level: String(pluginProbeForm.level || "info"),
    component: String(pluginProbeForm.component || "monitor.logs.ui"),
    trace_id: String(pluginProbeForm.traceId || `monitor-probe-${Date.now()}`),
    tenant_uuid: String(pluginProbeForm.tenantUuid || ""),
  };
}

function applyProbeFormFromPayload(payload: Record<string, any>) {
  pluginProbeForm.message = String(payload?.message || "monitor plugin logger probe");
  pluginProbeForm.level = String(payload?.level || "info");
  pluginProbeForm.component = String(payload?.component || "monitor.logs.ui");
  pluginProbeForm.traceId = String(payload?.trace_id || "monitor-probe-001");
  pluginProbeForm.tenantUuid = String(payload?.tenant_uuid || "");
}

function syncOrchJsonFromForm() {
  pluginLoggingOrch.policyJson = prettyJSON(buildPolicyPayloadFromForm());
  pluginLoggingOrch.probeJson = prettyJSON(buildProbePayloadFromForm());
}

function openPluginPolicyForm() {
  pluginPolicyFormOpen.value = true;
}

function openPluginProbeForm() {
  pluginProbeFormOpen.value = true;
}

function applyPluginPolicyForm() {
  syncOrchJsonFromForm();
  pluginPolicyFormOpen.value = false;
}

function applyPluginProbeForm() {
  if (!String(pluginProbeForm.traceId || "").trim()) {
    pluginProbeForm.traceId = `monitor-probe-${Date.now()}`;
  }
  syncOrchJsonFromForm();
  pluginProbeFormOpen.value = false;
}

function togglePluginAdvancedJson() {
  pluginAdvancedJsonOpen.value = !pluginAdvancedJsonOpen.value;
}

function applyAdvancedJsonToForms() {
  const policy = parseOrchJSON(pluginLoggingOrch.policyJson, "策略 JSON");
  const probe = parseOrchJSON(pluginLoggingOrch.probeJson, "Probe JSON");
  applyPolicyFormFromPayload(policy);
  applyProbeFormFromPayload(probe);
  pluginLoggingOrch.policyJson = prettyJSON(policy);
  pluginLoggingOrch.probeJson = prettyJSON(probe);
}

async function refreshPluginLoggingTargets() {
  try {
    await monitorLogsStore.fetchPluginTargets();
    if (!pluginLoggingOrch.pluginId) {
      pluginLoggingOrch.pluginId = String(monitorPluginTargets.value?.[0]?.plugin_id || "");
    }
    if (!pluginProbeForm.tenantUuid) {
      pluginProbeForm.tenantUuid = String(effectiveTenantUuid.value || "");
      syncOrchJsonFromForm();
    }
  } catch (e: any) {
    toast.add({ title: "读取插件列表失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function loadPluginLoggingPolicy() {
  if (!pluginLoggingOrch.pluginId) {
    toast.add({ title: "请先选择插件", color: "warning" });
    return;
  }
  try {
    const policy = await monitorLogsStore.fetchPluginPolicy(pluginLoggingOrch.pluginId);
    const normalized = (policy || {}) as Record<string, any>;
    applyPolicyFormFromPayload(normalized);
    syncOrchJsonFromForm();
    pluginLoggingOrch.resultJson = prettyJSON(normalized);
  } catch (e: any) {
    toast.add({ title: "读取插件策略失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function savePluginLoggingPolicy() {
  if (!pluginLoggingOrch.pluginId) {
    toast.add({ title: "请先选择插件", color: "warning" });
    return;
  }
  try {
    const payload = pluginAdvancedJsonOpen.value
      ? parseOrchJSON(pluginLoggingOrch.policyJson, "策略 JSON")
      : buildPolicyPayloadFromForm();
    const result = await monitorLogsStore.updatePluginPolicy(pluginLoggingOrch.pluginId, payload);
    const normalized = (result || {}) as Record<string, any>;
    applyPolicyFormFromPayload(normalized);
    syncOrchJsonFromForm();
    pluginLoggingOrch.resultJson = prettyJSON(normalized);
    toast.add({ title: "插件策略下发成功", color: "success" });
  } catch (e: any) {
    toast.add({ title: "下发插件策略失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function probePluginLoggingPolicy() {
  if (!pluginLoggingOrch.pluginId) {
    toast.add({ title: "请先选择插件", color: "warning" });
    return;
  }
  try {
    const payload = pluginAdvancedJsonOpen.value
      ? parseOrchJSON(pluginLoggingOrch.probeJson, "Probe JSON")
      : buildProbePayloadFromForm();
    const result = await monitorLogsStore.probePluginPolicy(pluginLoggingOrch.pluginId, payload);
    pluginLoggingOrch.probeJson = prettyJSON(payload);
    pluginLoggingOrch.resultJson = prettyJSON((result || {}) as Record<string, any>);
    toast.add({ title: "插件策略探测完成", color: "success" });
  } catch (e: any) {
    toast.add({ title: "执行插件 Probe 失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function queryMonitorLogs(resetPage = false) {
  try {
    const targetPage = resetPage ? 1 : (monitorLogsPage.value || 1);
    await monitorLogsStore.fetchLogs({
      driver: monitorLogDriverSelection.value === "auto" ? undefined : monitorLogDriverSelection.value,
      trace_id: monitorLogFilters.traceId || undefined,
      job_id: monitorLogFilters.jobId || undefined,
      policy_id: monitorLogFilters.policyId || undefined,
      keyword: monitorLogFilters.keyword || undefined,
      from: monitorLogFilters.from || undefined,
      to: monitorLogFilters.to || undefined,
      page: targetPage,
      page_size: monitorLogsPageSize.value || 50,
    });
    monitorLogPageSizeDraft.value = monitorLogsPageSize.value || 50;
  } catch (e: any) {
    toast.add({ title: "查询日志失败", description: e?.message || "未知错误", color: "error" });
  }
}

const monitorLogsTotalPages = computed(() => {
  const total = Number(monitorLogsTotal.value || 0);
  const size = Math.max(1, Number(monitorLogsPageSize.value || 50));
  return Math.max(1, Math.ceil(total / size));
});

const canMonitorLogPrevPage = computed(() => Number(monitorLogsPage.value || 1) > 1);
const canMonitorLogNextPage = computed(() => Number(monitorLogsPage.value || 1) < monitorLogsTotalPages.value);

async function prevMonitorLogsPage() {
  if (!canMonitorLogPrevPage.value) return;
  monitorLogsPage.value = Math.max(1, Number(monitorLogsPage.value || 1) - 1);
  await queryMonitorLogs(false);
}

async function nextMonitorLogsPage() {
  if (!canMonitorLogNextPage.value) return;
  monitorLogsPage.value = Number(monitorLogsPage.value || 1) + 1;
  await queryMonitorLogs(false);
}

async function applyMonitorLogPageSize() {
  const size = Number(monitorLogPageSizeDraft.value || 50);
  monitorLogsPageSize.value = [20, 50, 100].includes(size) ? size : 50;
  monitorLogsPage.value = 1;
  await queryMonitorLogs(false);
}

async function refreshMonitorRetentionRuns() {
  try {
    await monitorLogsStore.fetchRetentionRuns(20);
  } catch (e: any) {
    toast.add({ title: "查询日志保留任务失败", description: e?.message || "未知错误", color: "error" });
  }
}

function syncRetentionPolicyFormFromStore() {
  const policy = monitorRetentionPolicy.value;
  retentionPolicyForm.enabled = Boolean(policy?.enabled);
  retentionPolicyForm.cron = String(policy?.cron || "");
  retentionPolicyForm.timezone = String(policy?.timezone || "Asia/Shanghai");
  retentionPolicyForm.defaultRetentionDays = Number(policy?.default_retention_days || 30);
  retentionPolicyForm.batchSize = Number(policy?.batch_size || 5000);
  retentionPolicyForm.maxDeleteRowsPerRun = Number(policy?.max_delete_rows_per_run || 200000);
  retentionPolicyForm.filePathsText = Array.isArray(policy?.file_paths) ? policy.file_paths.join(",") : "";
  syncRetentionScheduleFromCron(retentionPolicyForm.cron);
}

function toIntInRange(value: any, min: number, max: number, fallback: number) {
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.floor(n)));
}

function buildCronFromRetentionSchedule(): string {
  if (retentionScheduleForm.mode === "minutes") {
    const every = toIntInRange(retentionScheduleForm.every, 1, 59, 15);
    return `*/${every} * * * *`;
  }
  if (retentionScheduleForm.mode === "hours") {
    const every = toIntInRange(retentionScheduleForm.every, 1, 23, 1);
    return `0 */${every} * * *`;
  }
  if (retentionScheduleForm.mode === "daily") {
    const hour = toIntInRange(retentionScheduleForm.dailyHour, 0, 23, 3);
    const minute = toIntInRange(retentionScheduleForm.dailyMinute, 0, 59, 10);
    return `${minute} ${hour} * * *`;
  }
  return String(retentionScheduleForm.customCron || "").trim();
}

function syncRetentionScheduleFromCron(cronRaw: string) {
  const cron = String(cronRaw || "").trim();
  retentionScheduleForm.customCron = cron;
  if (!cron) {
    retentionScheduleForm.mode = "daily";
    retentionScheduleForm.dailyHour = 3;
    retentionScheduleForm.dailyMinute = 10;
    return;
  }

  const minuteMatch = cron.match(/^\*\/(\d{1,2}) \* \* \* \*$/);
  if (minuteMatch) {
    retentionScheduleForm.mode = "minutes";
    retentionScheduleForm.every = toIntInRange(minuteMatch[1], 1, 59, 15);
    return;
  }

  const hourMatch = cron.match(/^0 \*\/(\d{1,2}) \* \* \*$/);
  if (hourMatch) {
    retentionScheduleForm.mode = "hours";
    retentionScheduleForm.every = toIntInRange(hourMatch[1], 1, 23, 1);
    return;
  }

  if (cron === "0 * * * *") {
    retentionScheduleForm.mode = "hours";
    retentionScheduleForm.every = 1;
    return;
  }

  const dailyMatch = cron.match(/^(\d{1,2}) (\d{1,2}) \* \* \*$/);
  if (dailyMatch) {
    retentionScheduleForm.mode = "daily";
    retentionScheduleForm.dailyMinute = toIntInRange(dailyMatch[1], 0, 59, 10);
    retentionScheduleForm.dailyHour = toIntInRange(dailyMatch[2], 0, 23, 3);
    return;
  }

  retentionScheduleForm.mode = "custom";
}

async function refreshMonitorRetentionPolicy() {
  try {
    await monitorLogsStore.fetchRetentionPolicy();
    syncRetentionPolicyFormFromStore();
  } catch (e: any) {
    toast.add({ title: "读取日志清理策略失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function saveMonitorRetentionPolicy() {
  if (!retentionPolicyForm.cron.trim()) {
    toast.add({ title: "保存失败", description: "cron 不能为空", color: "warning" });
    return;
  }
  if (!retentionPolicyForm.timezone.trim()) {
    toast.add({ title: "保存失败", description: "timezone 不能为空", color: "warning" });
    return;
  }
  const policy = monitorRetentionPolicy.value;
  try {
    await monitorLogsStore.updateRetentionPolicy({
      enabled: retentionPolicyForm.enabled,
      cron: retentionPolicyForm.cron.trim(),
      timezone: retentionPolicyForm.timezone.trim(),
      default_retention_days: Math.max(1, Number(retentionPolicyForm.defaultRetentionDays || 30)),
      batch_size: Math.max(1, Number(retentionPolicyForm.batchSize || 5000)),
      max_delete_rows_per_run: Math.max(1, Number(retentionPolicyForm.maxDeleteRowsPerRun || 200000)),
      file_paths: String(retentionPolicyForm.filePathsText || "")
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean),
      db_tables: Array.isArray(policy?.db_tables) ? policy.db_tables : [],
    });
    syncRetentionPolicyFormFromStore();
    toast.add({ title: "日志清理策略已保存", color: "success" });
  } catch (e: any) {
    toast.add({ title: "保存日志清理策略失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function triggerMonitorRetentionRun() {
  try {
    const run = await monitorLogsStore.triggerRetentionRun();
    toast.add({
      title: "日志保留任务已执行",
      description: `status=${run.status}, deleted_files=${run.deleted_files}, deleted_rows=${run.deleted_rows}`,
      color: run.status === "success" ? "success" : "warning",
    });
  } catch (e: any) {
    toast.add({ title: "执行日志保留任务失败", description: e?.message || "未知错误", color: "error" });
  }
}

async function triggerMonitorRetentionDryRun() {
  try {
    const days = Math.max(0, Number(retentionDryRunDays.value || 0));
    const run = await monitorLogsStore.triggerRetentionDryRun(days);
    toast.add({
      title: "日志清理试运行完成",
      description: `days=${run.retention_days ?? days}, matched_files=${run.deleted_files}, matched_rows=${run.deleted_rows}`,
      color: run.status === "success" ? "success" : "warning",
    });
  } catch (e: any) {
    toast.add({ title: "执行试运行失败", description: e?.message || "未知错误", color: "error" });
  }
}

function toggleRetentionRunDetails(runID: string) {
  retentionDetailRunID.value = retentionDetailRunID.value === runID ? "" : runID;
}

function downloadTextAsFile(content: string, filename: string, mimeType = "text/plain;charset=utf-8") {
  if (typeof window === "undefined") return;
  const blob = new Blob([content || ""], { type: mimeType });
  const url = window.URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename || `retention-export-${Date.now()}.txt`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.URL.revokeObjectURL(url);
}

async function exportRetentionDetails(item: { run_id?: string; retention_days?: number; cutoff_at?: string }, format: "txt" | "json") {
  const runID = String(item?.run_id || "");
  if (!runID) return;
  if (retentionExportLoadingMap[runID]) return;
  retentionExportLoadingMap[runID] = true;
  try {
    const exported = await monitorLogsStore.exportRetentionDryRun({
      format,
      retention_days: Number(item?.retention_days || 0),
      cutoff_at: String(item?.cutoff_at || ""),
    });
    const file = exported?.file;
    if (!file?.content) {
      throw new Error("导出内容为空");
    }
    downloadTextAsFile(file.content, file.name || `retention-hits.${format}`, file.mime_type || "text/plain;charset=utf-8");
    toast.add({
      title: "导出成功",
      description: `matched_files=${exported.matched_files ?? 0}, matched_rows=${exported.matched_rows ?? 0}`,
      color: "success",
    });
  } catch (e: any) {
    toast.add({ title: "导出失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    retentionExportLoadingMap[runID] = false;
  }
}

async function ensureMonitorLogsReady() {
  await refreshMonitorLogsConfig();
  await refreshPluginLoggingTargets();
  await refreshMonitorRetentionPolicy();
  await refreshMonitorRetentionRuns();
  monitorLogPageSizeDraft.value = monitorLogsPageSize.value || 50;
  if (!monitorLogsLoaded.value) {
    await queryMonitorLogs(true);
  }
}

function openMonitorGrafana() {
  if (!monitorGrafanaUrl.value) {
    toast.add({ title: "当前没有可用 Grafana 深链", color: "warning" });
    return;
  }
  window.open(monitorGrafanaUrl.value, "_blank", "noopener,noreferrer");
}


async function ensureTenantContextLoaded() {
  if (effectiveTenantUuid.value) return true;
  try {
    await userStore.fetchUserContext({ force: true });
  } catch {
  }
  return Boolean(effectiveTenantUuid.value);
}

const cronDebug = reactive<{
  loading: boolean;
  now: string;
  jobs: EventFabricCronJob[];
  logs: string[];
}>({
  loading: false,
  now: "",
  jobs: [],
  logs: [],
});
const cronLastRunAtMap = reactive<Record<string, string>>({});

const runtimeSchedulerStatusOptions = [
  { label: "全部状态", value: "all" },
  { label: "active", value: "active" },
  { label: "paused", value: "paused" },
  { label: "completed", value: "completed" },
  { label: "deleted", value: "deleted" },
];
const runtimeSchedulerFilters = reactive({
  ownerId: "",
  status: "all",
});
const runtimeScheduler = reactive<{
  loading: boolean;
  jobs: RuntimeSchedulerJob[];
  total: number;
  lastLoadedAt: string;
  lastAction: string;
}>({
  loading: false,
  jobs: [],
  total: 0,
  lastLoadedAt: "",
  lastAction: "",
});

const pushCronDebug = (msg: string) => {
  cronDebug.logs.unshift(`[${new Date().toLocaleTimeString()}] ${msg}`);
  cronDebug.logs = cronDebug.logs.slice(0, 60);
};

function formatTaskIDsForLog(taskIDs: string[]) {
  return taskIDs.length > 0 ? taskIDs.join(", ") : "无（本次未命中新任务）";
}

function toMillis(input?: string) {
  if (!input) return 0;
  const value = Date.parse(input);
  return Number.isFinite(value) ? value : 0;
}

async function listShardHistoryTaskIDs(tenantKey: string, subscriberID: string, limit = 50) {
  const msgRes = await svc.getTaskQueueMessages({
    tenant_key: tenantKey,
    subscriber_id: subscriberID,
    limit,
  });
  const history = msgRes.data?.history || [];
  const ids = history
    .map((item) => String(item?.task_id || "").trim())
    .filter(Boolean);
  return {
    messages: msgRes.data?.messages,
    history,
    ids,
  };
}

async function collectHistoryTaskIDsAcrossShards(limitPerShard = 30) {
  const statsRes = await svc.getTaskQueueStats();
  const rows = statsRes.data?.task_queue?.by_subscriber || [];
  const allIDs: string[] = [];
  for (const row of rows.slice(0, 40)) {
    if (!row?.tenant_key || !row?.subscriber_id) continue;
    const payload = await listShardHistoryTaskIDs(row.tenant_key, row.subscriber_id, limitPerShard);
    allIDs.push(...payload.ids);
  }
  return Array.from(new Set(allIDs));
}

async function collectRecentTaskIDsAcrossShards(sinceMs: number) {
  const statsRes = await svc.getTaskQueueStats();
  const rows = statsRes.data?.task_queue?.by_subscriber || [];
  const hits: Array<{ task_id: string; ts: number }> = [];
  for (const row of rows.slice(0, 40)) {
    if (!row?.tenant_key || !row?.subscriber_id) continue;
    const msgRes = await svc.getTaskQueueMessages({
      tenant_key: row.tenant_key,
      subscriber_id: row.subscriber_id,
      limit: 30,
    });
    const history = msgRes.data?.history || [];
    for (const item of history) {
      const taskID = String(item?.task_id || "").trim();
      if (!taskID) continue;
      const ts = Math.max(
        toMillis(item?.submitted_at),
        toMillis(item?.completed_at),
        toMillis(item?.last_seen_at),
      );
      if (ts >= sinceMs - 1000) {
        hits.push({ task_id: taskID, ts });
      }
    }
  }
  const unique = Array.from(new Map(hits.sort((a, b) => b.ts - a.ts).map((item) => [item.task_id, item])).values());
  return unique.slice(0, 5).map((item) => item.task_id);
}

async function loadCronJobsDebug() {
  cronDebug.loading = true;
  try {
    const res = await svc.listCronJobs();
    cronDebug.jobs = res.data.items || [];
    cronDebug.now = res.data.now || "";
    pushCronDebug(`list ok: jobs=${cronDebug.jobs.length}`);
  } catch (e: any) {
    pushCronDebug(`list failed: ${e?.message || e}`);
    toast.add({ title: "加载 Cron 任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    cronDebug.loading = false;
  }
}

async function runCronJobNowDebug(jobId: string) {
  cronDebug.loading = true;
  try {
    const runStartedAt = Date.now();
    let beforeFixedShardIDs = new Set<string>();
    let beforeAllIDs = new Set<string>();
    if (jobId === "event_fabric.retry_dispatch") {
      beforeAllIDs = new Set(await collectHistoryTaskIDsAcrossShards(30));
    } else {
      const currentJob = cronDebug.jobs.find((item) => item.id === jobId);
      if (currentJob?.subscriber_id && currentJob?.tenant_key) {
        const before = await listShardHistoryTaskIDs(currentJob.tenant_key, currentJob.subscriber_id, 30);
        beforeFixedShardIDs = new Set(before.ids);
      }
    }

    const res = await svc.runCronJobNow(jobId);
    const job = res.data;
    cronLastRunAtMap[job.id] = new Date().toLocaleTimeString();
    pushCronDebug(`run-now ok: ${job.id} status=${job.status}`);
    if (job.subscriber_id && job.tenant_key) {
      const queueRes = await listShardHistoryTaskIDs(job.tenant_key, job.subscriber_id, 30);
      const runtime = queueRes.messages;
      const runtimeCount = (runtime?.pending?.length || 0)
        + (runtime?.deferred?.length || 0)
        + (runtime?.processing?.length || 0)
        + (runtime?.inflight?.length || 0);
      const historyCount = queueRes.history?.length || 0;
      const recentTaskIDs = queueRes.history
        .filter((item) => Math.max(toMillis(item?.submitted_at), toMillis(item?.completed_at), toMillis(item?.last_seen_at)) >= runStartedAt - 1000)
        .map((item) => String(item?.task_id || "").trim())
        .filter(Boolean)
        .slice(0, 5);
      const newTaskIDs = queueRes.ids.filter((item) => !beforeFixedShardIDs.has(item)).slice(0, 5);
      pushCronDebug(`queue check: ${job.subscriber_id}/${job.tenant_key} runtime=${runtimeCount} history=${historyCount}`);
      pushCronDebug(`queue new_task_ids: ${formatTaskIDsForLog(newTaskIDs)}`);
      pushCronDebug(`queue recent_task_ids: ${formatTaskIDsForLog(recentTaskIDs)}`);
    } else {
      const afterAllIDs = new Set(await collectHistoryTaskIDsAcrossShards(30));
      const newTaskIDs = Array.from(afterAllIDs).filter((item) => !beforeAllIDs.has(item)).slice(0, 5);
      const recentTaskIDs = await collectRecentTaskIDsAcrossShards(runStartedAt);
      pushCronDebug(`queue check: ${job.id} 无固定 subscriber/tenant，已跨分片扫描`);
      pushCronDebug(`queue new_task_ids: ${formatTaskIDsForLog(newTaskIDs)}`);
      pushCronDebug(`queue recent_task_ids: ${formatTaskIDsForLog(recentTaskIDs)}`);
      if (job.id === "event_fabric.retry_dispatch" && taskDebug.taskId) {
        try {
          const retryStatusRes = await svc.getRetryTaskSeed(taskDebug.taskId);
          const retryStatus = retryStatusRes.data;
          pushCronDebug(
            `retry delivery status: delivery_id=${retryStatus.delivery_id} status=${retryStatus.status} acked_at=${retryStatus.acked_at || "-"} last_attempt_at=${retryStatus.last_attempt_at || "-"} last_error=${retryStatus.last_error_code || "-"} nack_reason=${retryStatus.nack_reason || "-"}`
          );
        } catch (statusErr: any) {
          pushCronDebug(`retry delivery status: query failed ${statusErr?.message || statusErr}`);
        }
      }
    }
    await loadCronJobsDebug();
  } catch (e: any) {
    pushCronDebug(`run-now failed: ${jobId} ${e?.message || e}`);
    toast.add({ title: "运行失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    cronDebug.loading = false;
  }
}

async function pauseCronJobDebug(jobId: string) {
  cronDebug.loading = true;
  try {
    const res = await svc.pauseCronJob(jobId);
    const job = res.data;
    pushCronDebug(`pause ok: ${job.id} status=${job.status}`);
    await loadCronJobsDebug();
  } catch (e: any) {
    pushCronDebug(`pause failed: ${jobId} ${e?.message || e}`);
    toast.add({ title: "暂停失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    cronDebug.loading = false;
  }
}

async function resumeCronJobDebug(jobId: string) {
  cronDebug.loading = true;
  try {
    const res = await svc.resumeCronJob(jobId);
    const job = res.data;
    pushCronDebug(`resume ok: ${job.id} status=${job.status}`);
    await loadCronJobsDebug();
  } catch (e: any) {
    pushCronDebug(`resume failed: ${jobId} ${e?.message || e}`);
    toast.add({ title: "恢复失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    cronDebug.loading = false;
  }
}

async function loadRuntimeSchedulerJobs() {
  runtimeScheduler.loading = true;
  try {
    const res = await svc.listRuntimeSchedulerJobs({
      owner_type: "plugin",
      owner_id: runtimeSchedulerFilters.ownerId.trim() || undefined,
      status: runtimeSchedulerFilters.status === "all" ? undefined : runtimeSchedulerFilters.status,
      page: 1,
      page_size: 50,
    });
    const payload = res.data;
    runtimeScheduler.jobs = payload.items || [];
    runtimeScheduler.total = Number(payload.pagination?.total ?? payload.total ?? runtimeScheduler.jobs.length);
    runtimeScheduler.lastLoadedAt = new Date().toLocaleTimeString();
    runtimeScheduler.lastAction = `list ok: jobs=${runtimeScheduler.jobs.length}`;
  } catch (e: any) {
    runtimeScheduler.lastAction = `list failed: ${e?.message || e}`;
    toast.add({ title: "加载 Runtime Scheduler 失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    runtimeScheduler.loading = false;
  }
}

function runtimeSchedulerJobID(job: RuntimeSchedulerJob) {
  return String(job.job_id || job.uuid || "").trim();
}

function runtimeSchedulerStatusColor(status: string) {
  const normalized = String(status || "").trim().toLowerCase();
  if (normalized === "active") return "success";
  if (normalized === "paused") return "warning";
  if (normalized === "completed") return "neutral";
  if (normalized === "deleted") return "error";
  return "neutral";
}

function formatRuntimeSchedulerType(job: RuntimeSchedulerJob) {
  const scheduleType = String(job.schedule_type || "").trim();
  const timezone = String(job.timezone || "").trim();
  if (!timezone) return scheduleType || "-";
  return `${scheduleType || "-"} / ${timezone}`;
}

function normalizeZeroTime(value?: string) {
  const raw = String(value || "").trim();
  if (!raw || raw.startsWith("0001-01-01")) return "-";
  return raw;
}

function canTriggerRuntimeScheduler(job: RuntimeSchedulerJob) {
  return runtimeSchedulerJobID(job) !== "" && String(job.status || "").trim().toLowerCase() === "active";
}

async function triggerRuntimeSchedulerJob(job: RuntimeSchedulerJob) {
  const jobID = runtimeSchedulerJobID(job);
  if (!jobID) return;
  runtimeScheduler.loading = true;
  try {
    await svc.triggerRuntimeSchedulerJob(jobID);
    runtimeScheduler.lastAction = `trigger ok: ${jobID}`;
    toast.add({ title: "Runtime Scheduler 已触发", description: jobID, color: "success" });
    await loadRuntimeSchedulerJobs();
  } catch (e: any) {
    runtimeScheduler.lastAction = `trigger failed: ${jobID}`;
    toast.add({ title: "触发 Runtime Scheduler 失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    runtimeScheduler.loading = false;
  }
}

async function pauseRuntimeSchedulerJob(job: RuntimeSchedulerJob) {
  const jobID = runtimeSchedulerJobID(job);
  if (!jobID) return;
  runtimeScheduler.loading = true;
  try {
    await svc.pauseRuntimeSchedulerJob(jobID);
    runtimeScheduler.lastAction = `pause ok: ${jobID}`;
    await loadRuntimeSchedulerJobs();
  } catch (e: any) {
    runtimeScheduler.lastAction = `pause failed: ${jobID}`;
    toast.add({ title: "暂停 Runtime Scheduler 失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    runtimeScheduler.loading = false;
  }
}

async function resumeRuntimeSchedulerJob(job: RuntimeSchedulerJob) {
  const jobID = runtimeSchedulerJobID(job);
  if (!jobID) return;
  runtimeScheduler.loading = true;
  try {
    await svc.resumeRuntimeSchedulerJob(jobID);
    runtimeScheduler.lastAction = `resume ok: ${jobID}`;
    await loadRuntimeSchedulerJobs();
  } catch (e: any) {
    runtimeScheduler.lastAction = `resume failed: ${jobID}`;
    toast.add({ title: "恢复 Runtime Scheduler 失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    runtimeScheduler.loading = false;
  }
}

const pushTaskDebug = (msg: string) => {
  taskDebug.logs.unshift(`[${new Date().toLocaleTimeString()}] ${msg}`);
  taskDebug.logs = taskDebug.logs.slice(0, 60);
};

function resetTaskQueueHit() {
  taskDebug.queueTenantKey = "";
  taskDebug.queueSubscriberId = "";
  taskDebug.queueHitState = "";
  taskDebug.queueHitSource = "";
}

function applyTaskQueueHit(hit: { tenant_key: string; subscriber_id: string; state: string; source: string }) {
  taskDebug.queueTenantKey = hit.tenant_key;
  taskDebug.queueSubscriberId = hit.subscriber_id;
  taskDebug.queueHitState = hit.state;
  taskDebug.queueHitSource = hit.source;
}

function normalizeTopicKey(topic: string) {
  return resolveReplayTopic(String(topic || "").trim());
}

async function locateTaskQueueHit(taskId: string, preferred?: { tenant_key?: string; subscriber_id?: string }) {
  if (!taskId) return false;
  const statsRes = await svc.getTaskQueueStats();
  const rows = statsRes.data?.task_queue?.by_subscriber || [];
  const candidates = [] as Array<{ tenant_key: string; subscriber_id: string }>;
  if (preferred?.tenant_key && preferred?.subscriber_id) {
    candidates.push({ tenant_key: preferred.tenant_key, subscriber_id: preferred.subscriber_id });
  }
  for (const row of rows) {
    if (!row?.tenant_key || !row?.subscriber_id) continue;
    if (candidates.some((item) => item.tenant_key === row.tenant_key && item.subscriber_id === row.subscriber_id)) continue;
    candidates.push({ tenant_key: row.tenant_key, subscriber_id: row.subscriber_id });
  }

  for (const candidate of candidates.slice(0, 30)) {
    const msgRes = await svc.getTaskQueueMessages({
      tenant_key: candidate.tenant_key,
      subscriber_id: candidate.subscriber_id,
      limit: 50,
    });
    const messages = msgRes.data?.messages;
    const history = msgRes.data?.history || [];
    if ((messages?.pending || []).some((item) => String(item?.id || "").trim() === taskId)) {
      applyTaskQueueHit({ ...candidate, state: "pending", source: "runtime" });
      return true;
    }
    if ((messages?.deferred || []).some((item) => String(item?.id || "").trim() === taskId)) {
      applyTaskQueueHit({ ...candidate, state: "deferred", source: "runtime" });
      return true;
    }
    if ((messages?.processing || []).some((item) => String(item?.id || "").trim() === taskId)) {
      applyTaskQueueHit({ ...candidate, state: "processing", source: "runtime" });
      return true;
    }
    if ((messages?.inflight || []).some((item) => String(item?.id || "").trim() === taskId)) {
      applyTaskQueueHit({ ...candidate, state: "inflight", source: "runtime" });
      return true;
    }
    const hit = history.find((item) => String(item?.task_id || "").trim() === taskId);
    if (hit) {
      applyTaskQueueHit({ ...candidate, state: String(hit.status || "history"), source: String(hit.source || "history") });
      return true;
    }
  }
  return false;
}

async function createTaskDebug() {
  if (!(await ensureTenantContextLoaded())) {
    toast.add({ title: "缺少租户上下文", description: "无法识别当前登录租户，请重新登录后重试", color: "warning" });
    return;
  }
  const topic = resolveReplayTopic(taskDebug.topic.trim());
  if (!topic) {
    toast.add({ title: "请先选择 topic", color: "warning" });
    return;
  }
  taskDebug.topic = topic;
  resetTaskQueueHit();
  taskDebug.loading = true;
  try {
    if (taskDebug.mode === "replay") {
      const res = await svc.createReplayTask({
        topic,
        trace_id: taskDebug.traceId.trim() || undefined,
        reason: "debug from monitor/task-cron",
        shadow: true,
      });
      const task = res.data;
      taskDebug.taskId = task.id || "";
      taskDebug.status = task.status || "";
      taskDebug.resultCount = task.result_count ?? null;
      taskDebug.failureReason = task.failure_reason || "";
      const preferred = {
        tenant_key: effectiveTenantUuid.value,
        subscriber_id: EVENT_SUBSCRIBERS.EVENT_FABRIC_REPLAY,
      };
      const hit = await locateTaskQueueHit(taskDebug.taskId, preferred);
      pushTaskDebug(`replay create ok: task_id=${taskDebug.taskId}, status=${taskDebug.status}, queue_hit=${hit ? "yes" : "no"}`);
      toast.add({ title: "Replay 任务创建成功", description: taskDebug.taskId, color: "success" });
      return;
    }
    if (taskDebug.mode === "pipeline") {
      const res = await svc.createPipelineTask({
        title: "Queue / Task 联调通知",
        content: "通过 Queue / Task 联调器触发的 Pipeline 任务",
        type: "system",
        category: "system",
        topic,
        subscriber_id: EVENT_SUBSCRIBERS.SYSTEM_NOTIFICATION_DISPATCH,
        tenant_key: "global",
        metadata: { source: "monitor.task-debugger" },
      });
      const result = res.data;
      taskDebug.taskId = String(result?.task_id || "");
      taskDebug.status = "queued";
      taskDebug.resultCount = null;
      taskDebug.failureReason = "";
      const preferred = {
        tenant_key: String(result?.tenant_key || "global"),
        subscriber_id: String(result?.subscriber_id || EVENT_SUBSCRIBERS.SYSTEM_NOTIFICATION_DISPATCH),
      };
      const hit = await locateTaskQueueHit(taskDebug.taskId, preferred);
      pushTaskDebug(`pipeline create ok: task_id=${taskDebug.taskId}, queue_hit=${hit ? "yes" : "no"}`);
      toast.add({ title: "Pipeline 任务创建成功", description: taskDebug.taskId, color: "success" });
      return;
    }

    const res = await svc.createRetryTaskSeed({
      topic,
      reason: "debug from monitor/task-cron",
    });
    const seed = res.data;
    taskDebug.taskId = String(seed.delivery_id || "");
    taskDebug.status = "retry_seeded";
    taskDebug.resultCount = null;
    taskDebug.failureReason = "";
    taskDebug.queueTenantKey = String(seed.tenant_key || "");
    taskDebug.queueSubscriberId = String(seed.subscriber_id || "");
    taskDebug.queueHitState = "scheduled";
    taskDebug.queueHitSource = "retry_seed";
    pushTaskDebug(`retry task created: delivery_id=${taskDebug.taskId}, event_id=${seed.event_id}, retry_at=${seed.retry_at}`);
    toast.add({
      title: "重试测试任务已创建",
      description: `retry_at=${seed.retry_at}`,
      color: "success",
    });
  } catch (e: any) {
    pushTaskDebug(`create failed: ${e?.message || e}`);
    toast.add({ title: "创建任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    taskDebug.loading = false;
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
  }
}

async function createRetrySampleTaskDebug() {
  if (!(await ensureTenantContextLoaded())) {
    toast.add({ title: "缺少租户上下文", description: "无法识别当前登录租户，请重新登录后重试", color: "warning" });
    return;
  }
  taskDebug.mode = "retry";
  const topic = resolveReplayTopic((taskDebug.topic || EVENT_TOPICS.SYSTEM_NOTIFICATION).trim()) || EVENT_TOPICS.SYSTEM_NOTIFICATION;
  taskDebug.topic = topic;
  taskDebug.loading = true;
  try {
    const res = await svc.createRetryTaskSeed({
      topic,
      reason: "debug from monitor/cron-quick-actions",
      immediate: false,
    });
    const seed = res.data;
    taskDebug.taskId = String(seed.delivery_id || "");
    taskDebug.status = "retry_seeded";
    taskDebug.resultCount = null;
    taskDebug.failureReason = "";
    taskDebug.queueTenantKey = String(seed.tenant_key || "");
    taskDebug.queueSubscriberId = String(seed.subscriber_id || "");
    taskDebug.queueHitState = "scheduled";
    taskDebug.queueHitSource = "retry_seed";
    pushTaskDebug(`retry task created: delivery_id=${taskDebug.taskId}, event_id=${seed.event_id}, retry_at=${seed.retry_at}, waiting_run_now=true`);
    pushCronDebug(`retry task created: delivery_id=${seed.delivery_id}, event_id=${seed.event_id}, retry_at=${seed.retry_at}, waiting_run_now=true`);
    toast.add({ title: "重试测试任务已创建", description: "任务已进入重试池，请在 Schedule 配置表格行点击“立即执行”", color: "success" });
  } catch (e: any) {
    pushTaskDebug(`retry task create failed: ${e?.message || e}`);
    pushCronDebug(`retry task create failed: ${e?.message || e}`);
    toast.add({ title: "创建重试测试任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    taskDebug.loading = false;
    queueStatsDirty.value = true;
    void syncQueueStatsIfDirty();
  }
}

async function runReplayQuickCheck() {
  if (taskDebug.mode !== "replay") {
    toast.add({ title: "仅 Replay 支持创建并查询", color: "info" });
    return;
  }
  await createTaskDebug();
  if (!taskDebug.taskId) return;
  await queryReplayTaskDebug();
}

async function queryReplayTaskDebug() {
  if (taskDebug.mode !== "replay") {
    toast.add({ title: "仅 Replay 支持查询", color: "info" });
    return;
  }
  const taskId = taskDebug.taskId.trim();
  if (!taskId) {
    toast.add({ title: "暂无 task_id", color: "warning" });
    return;
  }

  taskDebug.loading = true;
  try {
    const res = await svc.getReplayTask(taskId);
    const task = res.data;
    taskDebug.status = task.status || "";
    taskDebug.resultCount = task.result_count ?? null;
    taskDebug.failureReason = task.failure_reason || "";
    pushTaskDebug(`query ok: task_id=${taskId}, status=${taskDebug.status}, result=${taskDebug.resultCount ?? 0}`);
    toast.add({ title: "任务状态已刷新", description: taskDebug.status || "-", color: "success" });
  } catch (e: any) {
    pushTaskDebug(`query failed: ${e?.message || e}`);
    toast.add({ title: "查询任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    taskDebug.loading = false;
  }
}

function cronStatusColor(status: string) {
  const normalized = String(status || "").trim().toLowerCase();
  if (normalized === "running") return "success";
  if (normalized === "paused") return "warning";
  if (normalized === "unavailable") return "error";
  return "neutral";
}

function formatCronSchedule(job: EventFabricCronJob) {
  const kind = String(job.kind || "").trim().toLowerCase();
  const batch = Number(job.batch_size ?? 0);
  const batchText = `每批 ${Number.isFinite(batch) && batch > 0 ? batch : "-"} 条`;
  if (kind === "interval") {
    const interval = Number(job.interval_sec ?? 0);
    const intervalText = Number.isFinite(interval) && interval > 0 ? `${interval} 秒` : "-";
    return `每 ${intervalText} 触发 · ${batchText}`;
  }
  if (kind === "queue") {
    return `队列有待处理消息时消费 · ${batchText}`;
  }
  return `未配置 · ${batchText}`;
}

function formatSchedulerJobName(job: EventFabricCronJob) {
  const id = String(job.id || "").trim();
  if (id === "event_fabric.retry_dispatch") return "重试投递扫描";
  if (id === "event_fabric.authorization_challenge_timeout") return "授权超时处理";
  return String(job.name || id || "-");
}

function formatSchedulerKind(job: EventFabricCronJob) {
  const kind = String(job.kind || "").trim().toLowerCase();
  if (kind === "interval") return "定时扫描";
  if (kind === "queue") return "队列消费";
  return kind || "-";
}

function formatSchedulerTarget(job: EventFabricCronJob) {
  const id = String(job.id || "").trim();
  if (id === "event_fabric.retry_dispatch") return "扫描全部到期重试投递";
  if (id === "event_fabric.authorization_challenge_timeout") return "授权挑战超时队列";
  const subscriber = String(job.subscriber_id || "").trim();
  const tenant = String(job.tenant_key || "").trim();
  if (!subscriber && !tenant) return "-";
  return `${subscriber || "-"} / ${tenant || "-"}`;
}

async function cancelReplayTaskDebug() {
  if (taskDebug.mode !== "replay") {
    toast.add({ title: "仅 Replay 支持取消", color: "info" });
    return;
  }
  const taskId = taskDebug.taskId.trim();
  if (!taskId) {
    toast.add({ title: "暂无 task_id", color: "warning" });
    return;
  }

  taskDebug.loading = true;
  try {
    await svc.cancelReplayTask(taskId, {});
    pushTaskDebug(`cancel ok: task_id=${taskId}`);
    toast.add({ title: "已提交取消", description: taskId, color: "success" });
    await queryReplayTaskDebug();
  } catch (e: any) {
    pushTaskDebug(`cancel failed: ${e?.message || e}`);
    toast.add({ title: "取消任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    taskDebug.loading = false;
  }
}

watch(activeTab, (tab) => {
  if (tab === "websocket" && !wsTopic.value) {
    wsTopic.value = EVENT_TOPICS.SYSTEM_NOTIFICATION;
    void loadWsTopicCatalog();
  }
  if (tab === "task-cron" && cronDebug.jobs.length === 0 && !cronDebug.loading) {
    void loadCronJobsDebug();
    void loadRuntimeSchedulerJobs();
  }
  if (tab === "logs-trace") {
    void ensureMonitorLogsReady();
  }
});

watch(taskCronSubTab, (tab) => {
  if (activeTab.value !== "task-cron") return;
  if (tab === "platform" && cronDebug.jobs.length === 0 && !cronDebug.loading) {
    void loadCronJobsDebug();
  }
  if (tab === "runtime" && runtimeScheduler.jobs.length === 0 && !runtimeScheduler.loading) {
    void loadRuntimeSchedulerJobs();
  }
});

watch([activeTab, eventSubTab], () => {
  void syncQueueStatsIfDirty();
});

watch(
  () => taskDebug.mode,
  (mode) => {
    if (mode === "pipeline") {
      taskDebug.topic = EVENT_TOPICS.SYSTEM_NOTIFICATION;
    } else if (mode === "retry") {
      taskDebug.topic = EVENT_TOPICS.SYSTEM_NOTIFICATION;
    } else {
      taskDebug.topic = EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS;
    }
    taskDebug.taskId = "";
    taskDebug.status = "";
    taskDebug.resultCount = null;
    taskDebug.failureReason = "";
    resetTaskQueueHit();
  }
);

watch(
  () => [
    retentionScheduleForm.mode,
    retentionScheduleForm.every,
    retentionScheduleForm.dailyHour,
    retentionScheduleForm.dailyMinute,
    retentionScheduleForm.customCron,
  ],
  () => {
    retentionPolicyForm.cron = buildCronFromRetentionSchedule();
  }
);

watch(effectiveTenantUuid, (tenantUUID) => {
  if (!pluginProbeForm.tenantUuid) {
    pluginProbeForm.tenantUuid = String(tenantUUID || "");
    syncOrchJsonFromForm();
  }
});

onMounted(async () => {
  loadThresholdConfig();
  if (!pluginProbeForm.tenantUuid) {
    pluginProbeForm.tenantUuid = String(currentTenantUuid.value || "");
  }
  syncOrchJsonFromForm();
  activeTab.value = resolvedForcedTab.value || resolveTab(route.query.tab);
  await ensureTenantContextLoaded();
  await loadQueueTenants();
  ensureReplayStatusSubscription();
  if (activeTab.value === "event-fabric") {
    await refresh();
  }
  if (activeTab.value === "websocket") {
    await loadWsTopicCatalog();
  }
  if (activeTab.value === "task-cron") {
    await loadCronJobsDebug();
    await loadRuntimeSchedulerJobs();
  }
  if (activeTab.value === "logs-trace") {
    await ensureMonitorLogsReady();
  }
});

onBeforeUnmount(() => {
  stopFlowTemplateTask();
  if (wsDispose) {
    wsDispose();
    wsDispose = null;
  }
  if (replayStatusDispose) {
    replayStatusDispose();
    replayStatusDispose = null;
  }
});
</script>
