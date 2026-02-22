<template>
  <div class="p-6 space-y-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">监控中心</h1>
        <p class="text-gray-600 dark:text-gray-400">
          聚焦运行态 Queue 与联调，Topic 维护请前往事件管理页。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton icon="i-heroicons-arrow-path" :loading="loading" @click="refresh">刷新</UButton>
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
      <UCard>
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
                当前租户：
                <span class="font-mono">{{ effectiveTenantName }}</span>
              </div>
              <div class="text-sm text-gray-600 dark:text-gray-400">
                更新时间：
                <span class="font-mono">{{ overview?.now || '-' }}</span>
              </div>
            </div>
          </template>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
            <UInput v-model="filters.namespace" icon="i-heroicons-tag" placeholder="事件域（例：_topic.knowledge.space.feedback）" />
            <UInput v-model="filters.name" icon="i-heroicons-hashtag" placeholder="事件名（例：reprocess）" />
            <UInput v-model="filters.subscriberId" icon="i-heroicons-identification" placeholder="订阅者ID（例：_subscriber.knowledge_space.reprocess）" />
            <UButton variant="outline" @click="refresh">应用筛选</UButton>
          </div>

          <div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500">
            <span>常用预设：</span>
            <UButton size="xs" variant="soft" @click="applyFilterPreset('feedback_reprocess')">反馈重处理</UButton>
            <UButton size="xs" variant="soft" @click="applyFilterPreset('corpus_check')">语料巡检</UButton>
            <UButton size="xs" variant="ghost" @click="applyFilterPreset('clear')">清空筛选</UButton>
          </div>

          <div class="mt-1 text-xs text-gray-500">
            说明：`事件域 + 事件名` 用来定位 topic，`订阅者ID` 用来看某个消费者的投递/队列情况。
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

          <div class="mt-3 text-xs text-gray-500">统计时间：<span class="font-mono">{{ taskQueueNow || '-' }}</span></div>

          <div class="mt-3 overflow-x-auto">
            <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
              <thead class="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th class="text-left px-3 py-2">subscriber_id</th>
                  <th class="text-left px-3 py-2">tenant_key</th>
                  <th class="text-left px-3 py-2">pending</th>
                  <th class="text-left px-3 py-2">deferred</th>
                  <th class="text-left px-3 py-2">processing</th>
                  <th class="text-left px-3 py-2">inflight</th>
                  <th class="text-left px-3 py-2">total</th>
                  <th class="text-left px-3 py-2">任务清单</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in taskQueueRows"
                  :key="`${row.tenant_key}-${row.subscriber_id}`"
                  :class="['border-t border-gray-200 dark:border-gray-700', rowSeverityClass(row)]"
                >
                  <td class="px-3 py-2 font-mono">{{ row.subscriber_id }}</td>
                  <td class="px-3 py-2 font-mono">{{ row.tenant_key }}</td>
                  <td :class="['px-3 py-2', metricValueClass(row.pending, 'pending')]">{{ row.pending }}</td>
                  <td :class="['px-3 py-2', metricValueClass(row.deferred, 'deferred')]">{{ row.deferred }}</td>
                  <td :class="['px-3 py-2', metricValueClass(row.processing, 'processing')]">{{ row.processing }}</td>
                  <td :class="['px-3 py-2', metricValueClass(row.inflight, 'inflight')]">{{ row.inflight }}</td>
                  <td class="px-3 py-2 font-semibold">{{ rowTotalTasks(row) }}</td>
                  <td class="px-3 py-2">
                    <UButton size="xs" variant="outline" :loading="queueDetailLoading" @click="openQueueDetail(row)">
                      查看任务
                    </UButton>
                  </td>
                </tr>
                <tr v-if="taskQueueRows.length === 0">
                  <td class="px-3 py-3 text-gray-500" colspan="8">暂无 subscriber 统计数据</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="queueDetailVisible" class="mt-4 rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="text-sm">
                任务清单：
                <span class="font-mono">{{ queueDetailTenantKey }}</span>
                /
                <span class="font-mono">{{ queueDetailSubscriberID }}</span>
              </div>
              <div class="flex gap-2">
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
                任务历史
              </UButton>
            </div>

            <div class="overflow-x-auto">
              <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
                <thead class="bg-gray-50 dark:bg-gray-800/50">
                  <tr>
                    <th class="text-left px-3 py-2">task_id</th>
                    <th class="text-left px-3 py-2">topic</th>
                    <th class="text-left px-3 py-2">status</th>
                    <th class="text-left px-3 py-2">trace_id</th>
                    <th class="text-left px-3 py-2">attempt</th>
                    <th class="text-left px-3 py-2">topic_total</th>
                    <th class="text-left px-3 py-2">visible_at</th>
                    <th class="text-left px-3 py-2">submitted_at</th>
                    <th class="text-left px-3 py-2">completed_at</th>
                    <th class="text-left px-3 py-2">source</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="msg in queueUnifiedRows" :key="`${msg.source}-${msg.task_id}`" class="border-t border-gray-200 dark:border-gray-700">
                    <td class="px-3 py-2 font-mono">{{ msg.task_id }}</td>
                    <td class="px-3 py-2 font-mono">{{ formatTopicForDisplay(msg.topic) }}</td>
                    <td class="px-3 py-2">{{ msg.status }}</td>
                    <td class="px-3 py-2 font-mono">{{ msg.trace_id }}</td>
                    <td class="px-3 py-2">{{ msg.attempt }}</td>
                    <td class="px-3 py-2 font-semibold">{{ topicTaskCountMap[msg.topic] || 0 }}</td>
                    <td class="px-3 py-2 font-mono">{{ msg.visible_at }}</td>
                    <td class="px-3 py-2 font-mono">{{ msg.submitted_at }}</td>
                    <td class="px-3 py-2 font-mono">{{ msg.completed_at }}</td>
                    <td class="px-3 py-2">{{ msg.source }}</td>
                  </tr>
                  <tr v-if="queueUnifiedRows.length === 0">
                    <td class="px-3 py-3 text-gray-500" colspan="10">当前视图没有任务。</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="text-xs text-gray-500">
              说明：运行态来自 Redis 队列；任务历史来自 `event_task_histories` 持久化表。
            </div>
          </div>
        </UCard>

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
          <template #header><div class="font-semibold">Task / Cron 调试路径</div></template>
          <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300 space-y-1">
            <div class="font-semibold">建议顺序</div>
            <div>1) 先在「Task 联调」创建/查询 replay 任务，确认任务状态流转。</div>
            <div>2) 再在「Cron 管理」执行 run-now / pause / resume，确认调度控制有效。</div>
            <div>3) 队列容量阈值属于高级配置，放在页面底部折叠区。</div>
          </div>
        </UCard>

        <UCard>
          <template #header><div class="font-semibold">Replay Task 联调</div></template>

          <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300 space-y-1">
            <div class="font-semibold">接口能力：</div>
            <div>• 创建任务：<span class="font-mono">POST /admin/event-fabric/replay/tasks</span></div>
            <div>• 查询任务：<span class="font-mono">GET /admin/event-fabric/replay/tasks/:task_id</span></div>
            <div>• 取消任务：<span class="font-mono">POST /admin/event-fabric/replay/tasks/:task_id/cancel</span></div>
          </div>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-3 mt-3">
            <UInput v-model="taskDebug.topic" placeholder="topic（如 _topic.knowledge.space.feedback.reprocess）" class="md:col-span-2" />
            <UInput v-model="taskDebug.traceId" placeholder="trace_id（可选）" />
          </div>

          <div class="mt-3 flex flex-wrap gap-2">
            <UButton size="sm" color="primary" :loading="taskDebug.loading" @click="createReplayTaskDebug">创建任务</UButton>
            <UButton size="sm" variant="outline" :loading="taskDebug.loading" :disabled="!taskDebug.taskId" @click="queryReplayTaskDebug">
              查询任务
            </UButton>
            <UButton size="sm" variant="soft" color="warning" :loading="taskDebug.loading" :disabled="!taskDebug.taskId" @click="cancelReplayTaskDebug">
              取消任务
            </UButton>
          </div>

          <div class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono space-y-1">
            <div>current_task_id: {{ taskDebug.taskId || '-' }}</div>
            <div>status: {{ taskDebug.status || '-' }}</div>
            <div>result_count: {{ taskDebug.resultCount ?? '-' }}</div>
            <div>failure_reason: {{ taskDebug.failureReason || '-' }}</div>
          </div>

          <div class="mt-3 max-h-40 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono">
            <div v-for="(line, idx) in taskDebug.logs" :key="idx" class="mb-1">{{ line }}</div>
            <div v-if="taskDebug.logs.length === 0" class="text-gray-500">点击按钮开始 Task 联调。</div>
          </div>
        </UCard>

        <UCard>
          <template #header>
            <div class="flex items-center justify-between gap-2">
              <div class="font-semibold">Cron 管理</div>
              <UButton size="sm" variant="outline" :loading="cronDebug.loading" @click="loadCronJobsDebug">刷新任务</UButton>
            </div>
          </template>

          <UAlert
            icon="i-heroicons-information-circle"
            variant="subtle"
            title="说明"
            description="这里展示 Event Fabric 内部调度任务。你可以直接 run-now / pause / resume，用于联调任务机制。"
          />

          <div class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300">
            当前时间：<span class="font-mono">{{ cronDebug.now || '-' }}</span>
          </div>

          <div class="mt-3 overflow-x-auto">
            <table class="min-w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
              <thead class="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th class="text-left px-3 py-2">任务</th>
                  <th class="text-left px-3 py-2">类型</th>
                  <th class="text-left px-3 py-2">状态</th>
                  <th class="text-left px-3 py-2">周期/批次</th>
                  <th class="text-left px-3 py-2">subscriber/tenant</th>
                  <th class="text-left px-3 py-2">下次执行</th>
                  <th class="text-left px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="job in cronDebug.jobs" :key="job.id" class="border-t border-gray-200 dark:border-gray-700">
                  <td class="px-3 py-2">
                    <div class="font-medium">{{ job.name }}</div>
                    <div class="text-gray-500 font-mono">{{ job.id }}</div>
                  </td>
                  <td class="px-3 py-2">{{ job.kind || '-' }}</td>
                  <td class="px-3 py-2">{{ job.status }}</td>
                  <td class="px-3 py-2">
                    interval={{ job.interval_sec ?? '-' }}s · batch={{ job.batch_size ?? '-' }}
                  </td>
                  <td class="px-3 py-2 font-mono">
                    {{ job.subscriber_id || '-' }} / {{ job.tenant_key || '-' }}
                  </td>
                  <td class="px-3 py-2 font-mono">{{ job.next_run_at || '-' }}</td>
                  <td class="px-3 py-2">
                    <div class="flex flex-wrap gap-2">
                      <UButton size="xs" color="primary" :loading="cronDebug.loading" :disabled="!job.supports_run_now" @click="runCronJobNowDebug(job.id)">
                        run-now
                      </UButton>
                      <UButton size="xs" variant="outline" :loading="cronDebug.loading" :disabled="!job.supports_pause || job.status === 'paused'" @click="pauseCronJobDebug(job.id)">
                        pause
                      </UButton>
                      <UButton size="xs" variant="soft" color="success" :loading="cronDebug.loading" :disabled="!job.supports_pause || job.status !== 'paused'" @click="resumeCronJobDebug(job.id)">
                        resume
                      </UButton>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="mt-3 max-h-40 overflow-auto rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono">
            <div v-for="(line, idx) in cronDebug.logs" :key="idx" class="mb-1">{{ line }}</div>
            <div v-if="cronDebug.logs.length === 0" class="text-gray-500">点击“刷新任务”开始 Cron 联调。</div>
          </div>
        </UCard>

        <details class="rounded border border-gray-200 dark:border-gray-700 p-3">
          <summary class="cursor-pointer text-sm font-semibold">高级：监控阈值设置（任务队列高亮）</summary>
          <div class="mt-3">
            <div class="flex items-center justify-end gap-2 mb-3">
              <UButton size="xs" variant="outline" @click="resetThresholdConfig">重置默认</UButton>
              <UButton size="xs" color="primary" @click="saveThresholdConfig">保存阈值</UButton>
            </div>

            <div class="text-xs text-gray-500 mb-3">
              规则：数值 ≥ warn 且 &lt; danger 显示黄色；数值 ≥ danger 显示红色。
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
              <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                <div class="text-sm font-medium">pending</div>
                <UInput v-model.number="thresholdConfig.pendingWarn" type="number" min="0" placeholder="warn" />
                <UInput v-model.number="thresholdConfig.pendingDanger" type="number" min="0" placeholder="danger" />
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                <div class="text-sm font-medium">deferred</div>
                <UInput v-model.number="thresholdConfig.deferredWarn" type="number" min="0" placeholder="warn" />
                <UInput v-model.number="thresholdConfig.deferredDanger" type="number" min="0" placeholder="danger" />
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                <div class="text-sm font-medium">processing</div>
                <UInput v-model.number="thresholdConfig.processingWarn" type="number" min="0" placeholder="warn" />
                <UInput v-model.number="thresholdConfig.processingDanger" type="number" min="0" placeholder="danger" />
              </div>
              <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-2">
                <div class="text-sm font-medium">inflight</div>
                <UInput v-model.number="thresholdConfig.inflightWarn" type="number" min="0" placeholder="warn" />
                <UInput v-model.number="thresholdConfig.inflightDanger" type="number" min="0" placeholder="danger" />
              </div>
            </div>
          </div>
        </details>
      </div>

      <UCard v-else>
        <template #header><div class="font-semibold">Logs / Trace</div></template>
        <div class="space-y-3">
          <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs text-gray-600 dark:text-gray-300 space-y-1">
            <div class="font-semibold">当前实现可观测入口</div>
            <div>1) 实时链路：WebSocket 页签（消息里包含 topic/trace_id）。</div>
            <div>2) 任务链路：事件总线 Queue 详情（运行态 + 历史态）。</div>
            <div>3) 调度链路：Task/Cron 页签（run-now / pause / resume 操作日志）。</div>
          </div>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-2 text-xs">
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-1">
              <div class="text-gray-500">最近 WS topic</div>
              <div class="font-mono">{{ wsEvents[0]?.topic || '-' }}</div>
              <div class="text-gray-500 mt-2">最近 WS trace_id</div>
              <div class="font-mono">{{ wsEvents[0]?.traceId || '-' }}</div>
            </div>
            <div class="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-1">
              <div class="text-gray-500">最近 Replay task_id</div>
              <div class="font-mono">{{ taskDebug.taskId || flowDebug.taskId || '-' }}</div>
              <div class="text-gray-500 mt-2">最近阶段</div>
              <div class="font-mono">{{ flowDebug.phase || '-' }}</div>
            </div>
          </div>

          <div class="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs font-mono space-y-1 max-h-56 overflow-auto">
            <div v-for="(line, idx) in mergedTraceLogs" :key="idx">{{ line }}</div>
            <div v-if="mergedTraceLogs.length === 0" class="text-gray-500">暂无可展示日志，先去 WebSocket / Task-Cron 页签执行一次联调。</div>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useUserStore } from "~/stores/user";
import {
  useEventFabricService,
  type EventFabricOverview,
  type EventFabricCronJob,
  type EventFabricTaskQueueStats,
  type EventFabricTaskQueueMessage,
} from "~/composables/api/services/eventFabricService";
import { EVENT_NOTIFICATION_KIND, EVENT_SUBSCRIBERS, EVENT_TOPICS } from "~/composables/domain/eventTopic";
import { useWSBus } from "~/composables/useWSBus";

definePageMeta({
  title: "监控中心",
  layout: "default",
});

type MonitorTabKey = "event-fabric" | "websocket" | "task-cron" | "logs-trace";
type EventSubTabKey = "queue" | "debug";
const monitorTabs: Array<{ key: MonitorTabKey; label: string }> = [
  { key: "event-fabric", label: "事件总线" },
  { key: "websocket", label: "WebSocket" },
  { key: "task-cron", label: "Task / Cron" },
  { key: "logs-trace", label: "Logs / Trace" },
];
const eventSubTabs: Array<{ key: EventSubTabKey; label: string }> = [
  { key: "queue", label: "Queue" },
  { key: "debug", label: "联调" },
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
const activeTab = ref<MonitorTabKey>(resolveTab(route.query.tab));
const eventSubTab = ref<EventSubTabKey>("queue");
const setTab = (tab: MonitorTabKey) => {
  activeTab.value = tab;
};
const setEventSubTab = (tab: EventSubTabKey) => {
  eventSubTab.value = tab;
};

watch(activeTab, async (tab) => {
  await router.replace({ query: { ...route.query, tab } });
  if (tab === "event-fabric") {
    if (!eventSubTabs.some((item) => item.key === eventSubTab.value)) {
      eventSubTab.value = "queue";
    }
    await refresh();
  }
});

const svc = useEventFabricService();
const toast = useToast();

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
const taskQueueRows = computed(() => taskQueueStats.value?.by_subscriber || []);
const queueDetailLoading = ref(false);
const queueDetailVisible = ref(false);
const queueDetailTenantKey = ref("");
const queueDetailSubscriberID = ref("");
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
  topic: string;
  kind: string;
  status: string;
  trace_id?: string;
  attempt: number;
  source: string;
  submitted_at?: string;
  completed_at?: string;
  last_seen_at?: string;
}>>([]);
const currentQueueDetailRows = computed(() => queueDetailMessages[queueDetailState.value] || []);
const queueHistoryRows = computed(() => {
  return queueDetailHistory.value.map((item) => ({
    source: item.source || "history",
    task_id: item.task_id || "-",
    topic: item.topic || "-",
    status: item.status || "-",
    trace_id: item.trace_id || "-",
    attempt: String(item.attempt ?? 0),
    visible_at: item.last_seen_at || "-",
    submitted_at: item.submitted_at || "-",
    completed_at: item.completed_at || "-",
  }));
});
const queueRuntimeRows = computed(() =>
  currentQueueDetailRows.value.map((msg) => ({
    source: "runtime",
    task_id: msg.id || "-",
    topic: msg.topic || "-",
    status: queueDetailState.value,
    trace_id: msg.trace_id || "-",
    attempt: String(msg.attempt ?? 0),
    visible_at: msg.visible_at || "-",
    submitted_at: "-",
    completed_at: "-",
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
  toast.add({ title: "阈值已保存", color: "success" });
}

function resetThresholdConfig() {
  assignThresholdConfig(defaultThresholdConfig);
  if (typeof window !== "undefined") {
    window.localStorage.removeItem(thresholdStorageKey);
  }
  toast.add({ title: "已恢复默认阈值", color: "info" });
}

async function loadTaskQueueStats(showToast = false) {
  taskQueueLoading.value = true;
  try {
    const res = await svc.getTaskQueueStats({
      subscriber_id: filters.subscriberId || undefined,
    });
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
    });
    const payload = res.data?.messages;
    queueDetailMessages.pending = payload?.pending || [];
    queueDetailMessages.deferred = payload?.deferred || [];
    queueDetailMessages.processing = payload?.processing || [];
    queueDetailMessages.inflight = payload?.inflight || [];
    queueDetailHistory.value = res.data?.history || [];
  } catch (e: any) {
    if (showToast) {
      toast.add({ title: "加载任务清单失败", description: e?.message || "未知错误", color: "error" });
    }
  } finally {
    queueDetailLoading.value = false;
  }
}

async function openQueueDetail(row: { tenant_key: string; subscriber_id: string }) {
  queueDetailTenantKey.value = String(row.tenant_key || "").trim();
  queueDetailSubscriberID.value = String(row.subscriber_id || "").trim();
  queueDetailState.value = "pending";
  queueTaskViewMode.value = "runtime";
  queueDetailVisible.value = true;
  await loadQueueDetailMessages(queueDetailTenantKey.value, queueDetailSubscriberID.value, true);
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

  const topicInput = (taskDebug.topic || EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS).trim();
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
  topic: string;
  traceId: string;
  taskId: string;
  status: string;
  resultCount: number | null;
  failureReason: string;
  loading: boolean;
  logs: string[];
}>({
  topic: EVENT_TOPICS.KNOWLEDGE_FEEDBACK_REPROCESS,
  traceId: "",
  taskId: "",
  status: "",
  resultCount: null,
  failureReason: "",
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

const mergedTraceLogs = computed(() => {
  const lines: string[] = [];
  if (wsEvents.value.length > 0) {
    const ws = wsEvents.value[0];
    lines.push(`[WS] topic=${ws.topic} trace_id=${ws.traceId || "-"} ts=${ws.ts}`);
  }
  if (flowDebug.timeline.length > 0) {
    const flow = flowDebug.timeline[0];
    lines.push(`[FLOW] ${flow.ts} ${flow.stage} ${flow.detail}`);
  }
  if (taskDebug.logs.length > 0) {
    lines.push(`[TASK] ${taskDebug.logs[0]}`);
  }
  if (cronDebug.logs.length > 0) {
    lines.push(`[CRON] ${cronDebug.logs[0]}`);
  }
  return lines;
});


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

const pushCronDebug = (msg: string) => {
  cronDebug.logs.unshift(`[${new Date().toLocaleTimeString()}] ${msg}`);
  cronDebug.logs = cronDebug.logs.slice(0, 60);
};

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
    const res = await svc.runCronJobNow(jobId);
    const job = res.data;
    pushCronDebug(`run-now ok: ${job.id} status=${job.status}`);
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

const pushTaskDebug = (msg: string) => {
  taskDebug.logs.unshift(`[${new Date().toLocaleTimeString()}] ${msg}`);
  taskDebug.logs = taskDebug.logs.slice(0, 60);
};

async function createReplayTaskDebug() {
  const topic = resolveReplayTopic(taskDebug.topic.trim());
  if (!topic) {
    toast.add({ title: "请先输入 topic", color: "warning" });
    return;
  }
  taskDebug.topic = topic;

  taskDebug.loading = true;
  try {
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
    pushTaskDebug(`create ok: task_id=${taskDebug.taskId}, status=${taskDebug.status}`);
    toast.add({ title: "任务创建成功", description: taskDebug.taskId, color: "success" });
  } catch (e: any) {
    pushTaskDebug(`create failed: ${e?.message || e}`);
    toast.add({ title: "创建任务失败", description: e?.message || "未知错误", color: "error" });
  } finally {
    taskDebug.loading = false;
  }
}

async function queryReplayTaskDebug() {
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

async function cancelReplayTaskDebug() {
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
  }
});

watch([activeTab, eventSubTab], () => {
  void syncQueueStatsIfDirty();
});

onMounted(async () => {
  loadThresholdConfig();
  activeTab.value = resolveTab(route.query.tab);
  await ensureTenantContextLoaded();
  ensureReplayStatusSubscription();
  if (activeTab.value === "event-fabric") {
    await refresh();
  }
  if (activeTab.value === "websocket") {
    await loadWsTopicCatalog();
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
