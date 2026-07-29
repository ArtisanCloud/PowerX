<template>
  <div class="workflow-editor" :class="{ dark: isDark }" :style="workflowEditorStyle">
    <header class="workflow-topbar">
      <div class="workflow-title-group">
        <UButton
          icon="i-heroicons-arrow-left"
          color="neutral"
          variant="ghost"
          size="sm"
          to="/workflow"
          :aria-label="t('common.back')"
        />
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h1 class="workflow-title">{{ workflowDisplayName }}</h1>
            <UBadge v-if="currentWorkflow?.status" color="success" variant="soft">
              {{ t(`workflow.status.${currentWorkflow.status}`) }}
            </UBadge>
          </div>
          <div class="workflow-subtitle">
            <span>{{ t("workflow.editor.version", { version: currentWorkflow?.version || "-" }) }}</span>
            <span>{{ t("workflow.editor.nodeCount", { count: nodes.length }) }}</span>
            <span>{{ t("workflow.editor.edgeCount", { count: edges.length }) }}</span>
          </div>
        </div>
      </div>

      <UInput
        icon="i-heroicons-magnifying-glass"
        :placeholder="t('workflow.editor.globalSearch')"
        class="workflow-global-search"
      />

      <div class="workflow-top-actions">
        <UButton
          icon="i-heroicons-sun"
          color="neutral"
          variant="ghost"
          size="sm"
          :aria-label="t('workflow.editor.toggleTheme')"
          @click="toggleTheme"
        />
        <UButton icon="i-heroicons-document-arrow-down" color="neutral" variant="subtle" @click="handleSaveWorkflow">
          {{ t("workflow.editor.save") }}
        </UButton>
        <UButton
          icon="i-heroicons-play"
          color="neutral"
          variant="subtle"
          :loading="runLoading"
          :disabled="!currentWorkflow?.uuid"
          @click="openRunDialog"
        >
          {{ t("workflow.editor.runTest") }}
        </UButton>
        <UButton icon="i-heroicons-paper-airplane" color="primary">
          {{ t("workflow.editor.publish") }}
        </UButton>
      </div>
    </header>

    <section class="workflow-metrics">
      <div class="metric-cell">
        <span>{{ t("workflow.editor.metricNodes") }}</span>
        <strong>{{ nodes.length }}</strong>
      </div>
      <div class="metric-cell">
        <span>{{ t("workflow.editor.metricEdges") }}</span>
        <strong>{{ edges.length }}</strong>
      </div>
      <div class="metric-cell">
        <span>{{ t("workflow.editor.metricVersion") }}</span>
        <strong>{{ currentWorkflow?.version || "-" }}</strong>
      </div>
      <div class="metric-cell">
        <span>{{ t("workflow.editor.metricStatus") }}</span>
        <strong>{{ currentWorkflow?.status ? t(`workflow.status.${currentWorkflow.status}`) : "-" }}</strong>
      </div>
    </section>

    <div class="workflow-main">
      <aside class="workflow-palette">
        <div class="palette-header">
          <h3>{{ t("workflow.editor.componentLibrary") }}</h3>
        </div>
        <div class="palette-search">
          <UInput
            v-model="paletteSearch"
            icon="i-heroicons-magnifying-glass"
            :placeholder="t('workflow.editor.searchNodes')"
          />
        </div>
        <div class="palette-items">
          <section
            v-for="group in groupedPalette"
            :key="group.key"
            class="palette-group"
          >
            <div class="palette-group-title">{{ group.label }}</div>
            <div
              v-for="item in group.items"
              :key="item.id"
              class="palette-item"
              draggable="true"
              @dragstart="onDragStart($event, item.id)"
            >
              <div class="palette-item-icon" :class="`palette-icon-${group.key}`">
                <Icon :name="item.icon || 'i-heroicons-square-2-stack'" />
              </div>
              <div class="palette-item-content">
                <div class="palette-item-title">{{ displayLabel(item.label) }}</div>
                <div class="palette-item-kind">{{ getKindLabel(item.kind) }}</div>
              </div>
            </div>
          </section>
        </div>
      </aside>

      <main class="workflow-canvas-shell">
        <div class="canvas-toolbar">
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.undo')"
            :disabled="!canUndo"
            @click="undo"
          >
            <Icon name="i-heroicons-arrow-uturn-left" />
          </button>
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.redo')"
            :disabled="!canRedo"
            @click="redo"
          >
            <Icon name="i-heroicons-arrow-uturn-right" />
          </button>
          <span class="canvas-toolbar-separator" />
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.zoomOut')"
            @click="handleZoomOut"
          >
            <Icon name="i-heroicons-minus" />
          </button>
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.zoomIn')"
            @click="handleZoomIn"
          >
            <Icon name="i-heroicons-plus" />
          </button>
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.fitView')"
            @click="handleFitView"
          >
            <Icon name="i-heroicons-magnifying-glass" />
          </button>
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.centerView')"
            @click="handleCenterView"
          >
            <Icon name="i-heroicons-viewfinder-circle" />
          </button>
          <span class="canvas-toolbar-separator" />
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.resetZoom')"
            @click="handleResetZoom"
          >
            <Icon name="i-heroicons-arrows-pointing-in" />
          </button>
          <span class="canvas-toolbar-separator" />
          <button
            class="zoom-chip"
            type="button"
            :aria-label="t('workflow.editor.resetZoom')"
            @click="handleResetZoom"
          >
            {{ zoomPercent }}
          </button>
          <span class="canvas-toolbar-separator" />
          <button
            class="canvas-tool-button"
            type="button"
            :aria-label="t('workflow.editor.fullscreen')"
            @click="toggleFullscreen"
          >
            <Icon name="i-heroicons-arrows-pointing-out" />
          </button>
          <span class="canvas-toolbar-separator" />
          <button
            class="canvas-tool-button"
            :class="{ active: showMinimap }"
            type="button"
            :aria-label="t('workflow.editor.toggleMinimap')"
            @click="showMinimap = !showMinimap"
          >
            <Icon name="i-heroicons-rectangle-stack" />
          </button>
        </div>
        <VueFlow
          v-model:nodes="nodes"
          v-model:edges="edges"
          :default-viewport="{ x: 0, y: 0, zoom: 1 }"
          :min-zoom="0.2"
          :max-zoom="4"
          :nodes-focusable="false"
          :edges-focusable="false"
          :auto-pan-on-node-drag="false"
          :auto-pan-on-connect="false"
          class="workflow-vue-flow"
          @drop="onDrop"
          @dragover="onDragOver"
          @connect="handleConnect"
          @node-drag-stop="onNodeDragStop"
          @pane-ready="onReady"
          @node-click="onNodeClick"
        >
          <template #node-generic="nodeProps">
            <GenericNode
              v-bind="nodeProps"
              @update:props="updateNodeProps(nodeProps.id, $event)"
            />
          </template>

          <Background :pattern-color="isDark ? '#334155' : '#d7ddea'" :gap="18" />
          <MiniMap
            v-if="showMinimap"
            position="bottom-left"
            :width="176"
            :height="108"
            :pannable="true"
            :zoomable="true"
            :mask-border-radius="6"
            :node-border-radius="4"
            :node-stroke-width="1"
            :node-color="isDark ? '#334155' : '#d1d5db'"
            :node-stroke-color="isDark ? '#475569' : '#cbd5e1'"
            :mask-color="isDark ? 'rgba(96, 165, 250, 0.16)' : 'rgba(37, 99, 235, 0.14)'"
            :mask-stroke-color="isDark ? '#60a5fa' : '#3b82f6'"
          />
          <Controls position="bottom-left" />
        </VueFlow>
      </main>

      <aside class="workflow-properties">
        <div class="properties-tabs">
          <button
            class="properties-tab"
            :class="{ active: propertiesTab === 'config' }"
            type="button"
            @click="propertiesTab = 'config'"
          >
            {{ t("workflow.editor.nodeConfig") }}
          </button>
          <button
            class="properties-tab"
            :class="{ active: propertiesTab === 'runtime' }"
            type="button"
            @click="propertiesTab = 'runtime'"
          >
            {{ t("workflow.editor.nodeRuntime") }}
          </button>
          <button
            class="properties-tab"
            :class="{ active: propertiesTab === 'help' }"
            type="button"
            @click="propertiesTab = 'help'"
          >
            {{ t("workflow.editor.nodeHelp") }}
          </button>
        </div>
        <div class="properties-content">
          <template v-if="selectedNode">
            <div class="properties-header">
              <div class="node-avatar">
                <Icon :name="selectedNode.data.ui?.icon || 'i-heroicons-square-3-stack-3d'" />
              </div>
              <div class="min-w-0">
                <h4>{{ displayLabel(selectedNode.data.label) }}</h4>
                <div class="properties-kind">
                  {{ getKindLabel(selectedNode.data.kind) }}
                </div>
              </div>
            </div>

            <USeparator />

            <div v-if="propertiesTab === 'config'" class="properties-form">
              <template v-if="selectedNode.data.kind === 'human.review'">
                <UFormField class="properties-field" :label="t('workflow.fields.review_type')">
                  <USelect
                    v-model="selectedNode.data.props.review_type"
                    :items="reviewTypeOptions"
                    value-key="value"
                    label-key="label"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.approver_roles')">
                  <UInput
                    :model-value="humanReviewRolesText"
                    :placeholder="t('workflow.editor.approverRolesPlaceholder')"
                    @update:model-value="updateHumanReviewRoles(String($event || ''))"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.review_payload_path')">
                  <UInput v-model="selectedNode.data.props.review_payload_path" />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.approved_route')">
                  <USelect
                    v-model="selectedNode.data.props.approved_route"
                    :items="nodeRouteOptions"
                    value-key="value"
                    label-key="label"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.rejected_route')">
                  <USelect
                    v-model="selectedNode.data.props.rejected_route"
                    :items="nodeRouteOptions"
                    value-key="value"
                    label-key="label"
                  />
                </UFormField>
              </template>

              <template v-else-if="selectedNode.data.kind === 'input.capture'">
                <UFormField class="properties-field" :label="t('workflow.fields.input_schema_ref')">
                  <UInput
                    v-model="selectedNode.data.props.input_schema_ref"
                    :placeholder="t('workflow.fields.input_schema_ref_placeholder')"
                  />
                </UFormField>
                <div class="properties-note">
                  {{ t("workflow.editor.inputCaptureConfigHint") }}
                </div>
                <div class="properties-switch-grid">
                  <label>
                    <USwitch v-model="selectedNode.data.props.source_policy.text" />
                    <span>{{ t('workflow.fields.source_text') }}</span>
                  </label>
                  <label>
                    <USwitch v-model="selectedNode.data.props.source_policy.form" />
                    <span>{{ t('workflow.fields.source_form') }}</span>
                  </label>
                </div>
                <UFormField class="properties-field" :label="t('workflow.fields.artifact_output_path')">
                  <UInput
                    v-model="selectedNode.data.props.artifact_output_path"
                    :placeholder="t('workflow.fields.artifact_output_path_placeholder')"
                  />
                </UFormField>
              </template>

              <template v-else-if="selectedNode.data.kind === 'capability.invoke'">
                <div v-if="isRuntimeTemplateValue(selectedNode.data.props.capability_id)" class="properties-note">
                  {{ t("workflow.editor.runtimeCapabilityHint") }}
                </div>
                <UFormField class="properties-field" :label="t('workflow.fields.capability_id')">
                  <UInput v-model="selectedNode.data.props.capability_id" />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.preferred_protocol')">
                  <USelect
                    v-model="selectedNode.data.props.preferred_protocol"
                    :items="protocolOptions"
                    value-key="value"
                    label-key="label"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.input_path')">
                  <UInput v-model="selectedNode.data.props.input_path" />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.output_path')">
                  <UInput v-model="selectedNode.data.props.output_path" />
                </UFormField>
              </template>

              <template v-else-if="selectedNode.data.kind === 'event.emit'">
                <UFormField class="properties-field" :label="t('workflow.fields.topic')">
                  <UInput v-model="selectedNode.data.props.topic" />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.payload_path')">
                  <UInput v-model="selectedNode.data.props.payload_path" />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.event_schema_ref')">
                  <UInput v-model="selectedNode.data.props.event_schema_ref" />
                </UFormField>
              </template>

              <template v-else>
                <template
                  v-for="(value, key) in selectedNode.data.props"
                  :key="key"
                >
                  <UFormField class="properties-field" :label="schemaFieldLabel(key)">
                    <template v-if="typeof value === 'boolean'">
                      <USwitch v-model="selectedNode.data.props[key]" />
                    </template>
                    <template v-else-if="typeof value === 'number'">
                      <UInput
                        v-model.number="selectedNode.data.props[key]"
                        type="number"
                        :step="getNumberStep(key, selectedNode.data.schema)"
                      />
                    </template>
                    <template
                      v-else-if="isEnumField(key, selectedNode.data.schema)"
                    >
                      <USelect
                        v-model="selectedNode.data.props[key]"
                        :items="getEnumOptions(key, selectedNode.data.schema)"
                      />
                    </template>
                    <template
                      v-else-if="typeof value === 'string' && value.length > 50"
                    >
                      <UTextarea
                        v-model="selectedNode.data.props[key]"
                        :rows="5"
                      />
                    </template>
                    <template
                      v-else-if="typeof value === 'object' && value !== null"
                    >
                      <UTextarea
                        v-model="objectProps[key]"
                        :rows="5"
                        @blur="updateObjectProp(key)"
                      />
                    </template>
                    <template v-else>
                      <UInput v-model="selectedNode.data.props[key]" />
                    </template>
                  </UFormField>
                </template>
              </template>
            </div>
            <div v-else-if="propertiesTab === 'runtime'" class="node-runtime-panel">
              <div class="node-runtime-summary" :class="`state-${selectedNodeRunState}`">
                <span>{{ t("workflow.editor.currentNodeState") }}</span>
                <strong>{{ selectedNodeRunStateLabel }}</strong>
              </div>
              <template v-if="selectedNodeReviewTask">
                <div class="runtime-review-card">
                  <div>
                    <span>{{ t("workflow.editor.reviewRequest") }}</span>
                    <strong>{{ reviewTypeLabel(selectedNodeReviewTask.review_type) }}</strong>
                    <small>{{ stepDisplayName(selectedNodeReviewTask.step_id) }}</small>
                  </div>
                  <div class="runtime-review-actions">
                    <UButton
                      v-if="selectedNodeReviewTask.status === 'pending'"
                      color="success"
                      variant="soft"
                      icon="i-heroicons-check"
                      :loading="actingReviewTaskUUID === selectedNodeReviewTask.review_task_uuid && actingReviewAction === 'approve'"
                      @click="actReviewTask(selectedNodeReviewTask, 'approve')"
                    >
                      {{ t("workflow.review.approve") }}
                    </UButton>
                    <UButton
                      v-if="selectedNodeReviewTask.status === 'pending'"
                      color="error"
                      variant="soft"
                      icon="i-heroicons-x-mark"
                      :loading="actingReviewTaskUUID === selectedNodeReviewTask.review_task_uuid && actingReviewAction === 'reject'"
                      @click="actReviewTask(selectedNodeReviewTask, 'reject')"
                    >
                      {{ t("workflow.review.reject") }}
                    </UButton>
                  </div>
                </div>
                <div class="runtime-payload-card">
                  <span>{{ t("workflow.editor.reviewPayload") }}</span>
                  <pre>{{ formatReviewPayload(selectedNodeReviewTask.payload) }}</pre>
                </div>
              </template>
              <template v-else-if="selectedNodeRunStep">
                <div class="runtime-payload-card">
                  <span>{{ t("workflow.editor.nodeRunRecord") }}</span>
                  <pre>{{ formatStepRunPayload(selectedNodeRunStep) }}</pre>
                </div>
              </template>
              <div v-else-if="selectedNode.data.kind === 'human.review' && latestRunState === 'waiting'" class="runtime-empty-card">
                {{ t("workflow.editor.noReviewTaskForNode") }}
              </div>
              <div v-else class="runtime-empty-card">
                {{ t("workflow.editor.noRuntimeForNode") }}
              </div>
            </div>
            <div v-else class="node-help-panel">
              <div class="node-help-block">
                <span>{{ t("workflow.editor.nodeKind") }}</span>
                <strong>{{ getKindLabel(selectedNode.data.kind) }}</strong>
              </div>
              <div class="node-help-block">
                <span>{{ t("workflow.editor.requiredFields") }}</span>
                <div class="help-tags">
                  <span
                    v-for="field in selectedNode.data.schema?.required || []"
                    :key="field"
                  >
                    {{ schemaFieldLabel(field) }}
                  </span>
                  <span v-if="!(selectedNode.data.schema?.required || []).length">
                    {{ t("workflow.editor.none") }}
                  </span>
                </div>
              </div>
              <div class="node-help-block">
                <span>{{ t("workflow.editor.outputPorts") }}</span>
                <div class="help-tags">
                  <span
                    v-for="port in selectedNode.data.ports?.outputs || []"
                    :key="port.name"
                  >
                    {{ displayLabel(port.label || port.name) }}
                  </span>
                  <span v-if="!(selectedNode.data.ports?.outputs || []).length">
                    {{ t("workflow.editor.none") }}
                  </span>
                </div>
              </div>
            </div>
          </template>
          <div v-else class="properties-empty">
            <UIcon name="i-heroicons-cursor-arrow-rays" />
            <p>{{ t("workflow.editor.selectNodeHint") }}</p>
          </div>
        </div>
      </aside>
    </div>

    <footer class="workflow-bottom-panel">
      <div
        class="bottom-resize-handle"
        role="separator"
        aria-orientation="horizontal"
        :aria-label="t('workflow.editor.resizeBottomPanel')"
        @pointerdown="startBottomResize"
      />
      <div class="bottom-tabs">
        <button
          v-for="tab in bottomTabs"
          :key="tab.value"
          class="bottom-tab"
          :class="{ active: bottomTab === tab.value }"
          type="button"
          @click="bottomTab = tab.value"
        >
          {{ tab.label }}
        </button>
      </div>
      <div class="bottom-content">
        <template v-if="bottomTab === 'runs'">
          <div class="run-list">
            <div class="run-card" :class="latestRunCardClass">
              <span>{{ t("workflow.editor.latestRun") }}</span>
              <strong>{{ latestRunStateLabel }}</strong>
              <small v-if="latestRun?.uuid">{{ t("workflow.editor.instanceShort", { uuid: shortUUID(latestRun.uuid) }) }}</small>
            </div>
          </div>
          <div class="run-detail">
            <div class="run-detail-header">
              <strong>{{ t("workflow.editor.runDetail") }}</strong>
              <UBadge :color="latestRunBadgeColor" variant="soft">{{ latestRunStateLabel }}</UBadge>
              <UBadge :color="workflowRuntimeConnectionColor" variant="subtle">
                {{ workflowRuntimeConnectionLabel }}
              </UBadge>
              <UButton
                v-if="!workflowRuntimeBus.connected.value"
                size="xs"
                color="neutral"
                variant="soft"
                icon="i-heroicons-arrow-path"
                :loading="workflowRuntimeBus.connecting.value"
                @click="workflowRuntimeBus.connect()"
              >
                {{ t("workflow.editor.runtimeReconnect") }}
              </UButton>
              <UButton
                v-if="canCancelLatestRun"
                size="xs"
                color="error"
                variant="soft"
                icon="i-heroicons-stop-circle"
                :loading="cancelRunLoading"
                @click="cancelLatestRun"
              >
                {{ t("workflow.editor.cancelRun") }}
              </UButton>
            </div>
            <div v-if="runError" class="run-error">
              {{ runError }}
            </div>
            <div v-if="latestRun?.trace_id" class="run-meta">
              <span>{{ t("workflow.editor.traceId", { trace: latestRun.trace_id }) }}</span>
            </div>
            <div class="run-step-list" :aria-label="t('workflow.editor.stepList')">
              <div
                v-for="step in latestRunSteps"
                :key="step.id"
                class="run-step-row"
                :class="step.stateClass"
              >
                <div class="run-step-marker" aria-hidden="true" />
                <div class="run-step-main">
                  <strong>{{ step.label }}</strong>
                  <small>{{ step.kindLabel }}</small>
                  <span v-if="step.message">{{ step.message }}</span>
                </div>
                <UBadge class="run-step-status" :color="step.badgeColor" variant="soft">
                  {{ step.stateLabel }}
                </UBadge>
                <UButton
                  class="run-step-action"
                  size="xs"
                  color="neutral"
                  variant="ghost"
                  icon="i-heroicons-eye"
                  :aria-label="t('workflow.editor.viewNode')"
                  @click="selectRunStep(step.id)"
                >
                  {{ t("workflow.editor.viewNode") }}
                </UButton>
              </div>
            </div>
          </div>
        </template>
        <template v-else-if="bottomTab === 'logs'">
          <div class="run-logs">
            <h3>{{ t("workflow.editor.logs") }}</h3>
            <div v-if="latestRunLogs.length" class="run-log-list">
              <div v-for="item in latestRunLogs" :key="item.id" class="run-log-row" :class="item.stateClass">
                <div class="run-log-main">
                  <strong>{{ item.title }}</strong>
                  <small>{{ item.kindLabel }}</small>
                  <span>{{ item.message }}</span>
                </div>
                <div class="run-log-id" role="button" tabindex="0" @click="selectRunStep(item.stepID)" @keydown.enter="selectRunStep(item.stepID)">
                  <code>{{ item.stepID }}</code>
                  <UButton
                    size="xs"
                    color="neutral"
                    variant="ghost"
                    icon="i-heroicons-clipboard-document"
                    :aria-label="t('workflow.editor.copyStepId')"
                    @click.stop="copyToClipboard(item.stepID)"
                  />
                </div>
                <code v-if="item.error">{{ item.error }}</code>
              </div>
            </div>
            <p v-else>{{ t("workflow.editor.noLogs") }}</p>
          </div>
        </template>
        <template v-else-if="bottomTab === 'snapshots'">
          <div class="run-logs">
            <h3>{{ t("workflow.editor.variableSnapshots") }}</h3>
            <p>{{ t("workflow.editor.noSnapshots") }}</p>
          </div>
        </template>
        <template v-else>
          <div class="run-logs">
            <h3>{{ t("workflow.editor.evaluationResults") }}</h3>
            <p>{{ t("workflow.editor.noEvaluations") }}</p>
          </div>
        </template>
      </div>
    </footer>

    <UModal
      v-model:open="runDialogOpen"
      :title="t('workflow.editor.runDialogTitle')"
      :description="t('workflow.editor.runDialogDescription')"
      :ui="{ content: 'max-w-5xl w-[88vw]' }"
    >
      <template #body>
        <div class="run-dialog-body">
          <div class="run-dialog-summary">
            <div>
              <span>{{ t("workflow.editor.workflowToRun") }}</span>
              <strong>{{ workflowDisplayName }}</strong>
            </div>
            <div class="run-dialog-status">
              <UBadge :color="workflowRuntimeConnectionColor" variant="subtle">
                {{ workflowRuntimeConnectionLabel }}
              </UBadge>
              <UButton
                v-if="!workflowRuntimeBus.connected.value"
                size="xs"
                color="neutral"
                variant="soft"
                icon="i-heroicons-arrow-path"
                :loading="workflowRuntimeBus.connecting.value"
                @click="workflowRuntimeBus.connect()"
              >
                {{ t("workflow.editor.runtimeReconnect") }}
              </UButton>
            </div>
          </div>
          <div v-if="isApprovalGuardedCapabilityWorkflow" class="debug-input-form">
            <UAlert
              class="run-dialog-notice"
              icon="i-heroicons-information-circle"
              color="info"
              variant="subtle"
              :title="t('workflow.editor.workflowCapabilitySourceTitle')"
              :description="t('workflow.editor.workflowCapabilitySourceDescription')"
            />
            <UFormField :label="t('workflow.editor.runCapabilityModuleLabel')">
              <USelectMenu
                v-model="selectedRunCapabilityModule"
                :items="capabilityModuleSelectItems"
                label-key="label"
                :portal="runDialogSelectPortal"
                :content="runDialogSelectContent"
                :ui="runDialogSelectUi"
                class="w-full"
                :loading="capabilityOptionsLoading"
                :disabled="capabilityOptionsLoading || capabilityModuleSelectItems.length === 0"
                :placeholder="t('workflow.editor.capabilityModuleSelectPlaceholder')"
                :search-input="{ placeholder: t('workflow.editor.capabilityModuleSearchPlaceholder') }"
              />
              <div class="run-field-hint">
                {{ t("workflow.editor.capabilityModuleSelectHint") }}
              </div>
            </UFormField>
            <UFormField :label="t('workflow.editor.runCapabilityLabel')">
              <USelectMenu
                v-model="selectedRunCapability"
                :items="capabilitySelectItems"
                label-key="label"
                :portal="runDialogSelectPortal"
                :content="runDialogSelectContent"
                :ui="runDialogSelectUi"
                class="w-full"
                :loading="capabilityOptionsLoading"
                :disabled="capabilityOptionsLoading || !selectedRunCapabilityModule || capabilitySelectItems.length === 0"
                :placeholder="t('workflow.editor.capabilitySelectPlaceholder')"
                :search-input="{ placeholder: t('workflow.editor.capabilitySearchPlaceholder') }"
              />
              <div class="run-field-hint">
                {{ t("workflow.editor.capabilitySelectHint") }}
              </div>
              <div v-if="selectedRunCapabilityDetail" class="run-capability-detail">
                {{ selectedRunCapabilityDetail }}
              </div>
            </UFormField>
            <UFormField :label="t('workflow.fields.reason')">
              <USelectMenu
                v-model="selectedExecutionReason"
                :items="executionReasonOptions"
                label-key="label"
                :portal="runDialogSelectPortal"
                :content="runDialogSelectContent"
                :ui="runDialogSelectUi"
                class="w-full"
                :placeholder="t('workflow.editor.executionReasonPlaceholder')"
                :search-input="{ placeholder: t('workflow.editor.executionReasonSearchPlaceholder') }"
              />
              <div class="run-field-hint">
                {{ t("workflow.editor.executionReasonHint") }}
              </div>
            </UFormField>
            <label class="debug-input-switch">
              <USwitch v-model="approvalDebugForm.dry_run" />
              <span>
                <strong>{{ t("workflow.fields.dry_run") }}</strong>
                <small>{{ t("workflow.editor.dryRunHint") }}</small>
              </span>
            </label>
            <UFormField :label="t('workflow.fields.note')">
              <UTextarea
                v-model="approvalDebugForm.note"
                :placeholder="t('workflow.editor.debugNotePlaceholder')"
                :rows="3"
                class="w-full"
              />
            </UFormField>
            <div v-if="capabilityOptionsError" class="run-dialog-error">
              {{ capabilityOptionsError }}
            </div>
          </div>
          <UTextarea
            v-else
            v-model="debugInputText"
            :rows="8"
            spellcheck="false"
            class="debug-input-textarea"
          />
        </div>
      </template>
      <template #footer>
        <div class="run-dialog-footer">
          <UButton color="neutral" variant="subtle" @click="runDialogOpen = false">
            {{ t("common.cancel") }}
          </UButton>
          <UButton
            color="neutral"
            variant="ghost"
            icon="i-heroicons-arrow-path"
            @click="resetDebugInput"
          >
            {{ t("workflow.editor.resetDebugInput") }}
          </UButton>
          <UButton
            color="primary"
            icon="i-heroicons-play"
            :loading="runLoading"
            :disabled="capabilityOptionsLoading"
            @click="runWorkflow"
          >
            {{ t("workflow.editor.startRun") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, reactive, watch, nextTick } from "vue";
import { useColorMode, useI18n, useRoute, useToast } from "#imports";
import { VueFlow, useVueFlow } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import type { Node, Edge, Connection } from "@vue-flow/core";
import type { WfNode, Edge as WorkflowEdge } from "~/types/workflow";

import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";
import { useWorkflowManager } from "~/composables/workflow/useWorkflowManager";
import GenericNode from "./nodes/GenericNode.vue";
import { useWorkflowService, type HumanReviewTask, type WorkflowInstance, type WorkflowStepRecord } from "~/composables/api/services/workflowService";
import { useWorkflowRuntimeBus, type WorkflowRuntimeEvent } from "~/composables/workflow/useWorkflowRuntimeBus";
import { PlatformCapabilityService, type PlatformCapability } from "~/composables/api/services/platformCapabilityService";

// 主题支持
const colorMode = useColorMode();
const isDark = computed(() => colorMode.value === "dark");
const { t, te } = useI18n();
const route = useRoute();
const toast = useToast();
const workflowService = useWorkflowService();
const workflowRuntimeBus = useWorkflowRuntimeBus();

// 工作流管理器
const {
  kinds,
  palette,
  currentWorkflow,
  loadNodeCatalog,
  loadWorkflow,
  addNodeFromPalette,
  saveWorkflow,
} = useWorkflowManager();

// Vue Flow 实例
const {
  findNode,
  addEdges,
  fitView,
  setViewport,
  viewport,
  zoomIn,
  zoomOut,
  zoomTo,
  project,
  addNodes,
} = useVueFlow();

// 状态
const nodes = ref<Node[]>([]);
const edges = ref<Edge[]>([]);
const paletteSearch = ref("");
const selectedNode = ref<Node | null>(null);
const objectProps = reactive<Record<string, string>>({});
const hasLoadedCanvas = ref(false);
const showMinimap = ref(true);
const runLoading = ref(false);
const cancelRunLoading = ref(false);
const runDialogOpen = ref(false);
const latestRun = ref<WorkflowInstance | null>(null);
const latestReviewTasks = ref<HumanReviewTask[]>([]);
const runError = ref("");
const debugInputText = ref("{}");
type RunCapabilityOption = PlatformCapability & {
  moduleDisplayName: string;
};
type SelectOption = {
  label: string;
  value: string;
};

const capabilityOptions = ref<RunCapabilityOption[]>([]);
const capabilityOptionsLoading = ref(false);
const capabilityOptionsError = ref("");
const selectedRunCapabilityModule = ref<SelectOption | null>(null);
const selectedRunCapability = ref<SelectOption | null>(null);
const selectedExecutionReason = ref<SelectOption | null>(null);
const actingReviewTaskUUID = ref("");
const actingReviewAction = ref("");
const approvalDebugForm = reactive({
  capability_id: "com.corex.metadata.dictionary.read",
  reason: "workflow_debug_approval_guarded_capability",
  dry_run: true,
  note: "",
});
const bottomPanelHeight = ref(260);
let stopBottomResize: (() => void) | null = null;
let unsubscribeWorkflowRuntime: (() => void) | null = null;

type PropertiesTab = "config" | "runtime" | "help";
type BottomTab = "runs" | "logs" | "snapshots" | "evaluations";

const propertiesTab = ref<PropertiesTab>("config");
const bottomTab = ref<BottomTab>("runs");

const bottomTabs = computed(() => [
  { value: "runs" as const, label: t("workflow.editor.runRecords") },
  { value: "logs" as const, label: t("workflow.editor.debugLogs") },
  { value: "snapshots" as const, label: t("workflow.editor.variableSnapshots") },
  { value: "evaluations" as const, label: t("workflow.editor.evaluationResults") },
]);

const nodeRouteOptions = computed(() => {
  const currentNodeID = selectedNode.value?.id || "";
  return nodes.value
    .filter((node) => node.id !== currentNodeID)
    .map((node) => ({
      label: stepDisplayName(node.id),
      value: node.id,
    }));
});

const reviewTypeOptions = computed(() => [
  { label: t("workflow.review.type.capabilityExecution"), value: "capability_execution" },
  { label: t("workflow.review.type.knowledgePublish"), value: "knowledge_publish" },
  { label: t("workflow.review.type.metadataClassification"), value: "metadata_classification" },
  { label: t("workflow.review.type.skillResult"), value: "skill_result" },
]);

const protocolOptions = computed(() => [
  { label: t("workflow.protocol.rest"), value: "rest" },
  { label: t("workflow.protocol.grpc"), value: "grpc" },
  { label: t("workflow.protocol.skill"), value: "skill" },
]);

const workflowRunnableCapabilities = computed(() =>
  [...capabilityOptions.value].filter((capability) => isWorkflowRunnableBusinessCapability(capability))
);

const capabilityModuleSelectItems = computed<SelectOption[]>(() => {
  const modules = new Map<string, { label: string; count: number }>();
  for (const capability of workflowRunnableCapabilities.value) {
    const moduleKey = capability.module || "corex";
    const current = modules.get(moduleKey);
    if (current) {
      current.count += 1;
      continue;
    }
    modules.set(moduleKey, {
      label: capabilityModuleLabel(capability),
      count: 1,
    });
  }
  return [...modules.entries()]
    .sort((left, right) => left[1].label.localeCompare(right[1].label))
    .map(([value, item]) => ({
      label: t("workflow.editor.capabilityModuleOptionLabel", { module: item.label, count: item.count }),
      value,
    }));
});

const capabilitySelectItems = computed<SelectOption[]>(() =>
  workflowRunnableCapabilities.value
    .filter((capability) => capability.module === selectedRunCapabilityModule.value?.value)
    .sort((left, right) => Number(hasLocalizedCapabilityName(right)) - Number(hasLocalizedCapabilityName(left)))
    .map((capability) => ({
      label: capabilityOptionLabel(capability),
      value: capability.capabilityId,
    }))
);

const executionReasonOptions = computed<SelectOption[]>(() => [
  {
    label: t("workflow.executionReason.workflowDebugApprovalGuardedCapability"),
    value: "workflow_debug_approval_guarded_capability",
  },
  {
    label: t("workflow.executionReason.permissionBoundaryTest"),
    value: "permission_boundary_test",
  },
  {
    label: t("workflow.executionReason.businessDryRun"),
    value: "business_dry_run",
  },
]);

const runDialogSelectContent = {
  side: "bottom" as const,
  sideOffset: 8,
  collisionPadding: 16,
  position: "popper" as const,
};

const runDialogSelectPortal = false;

const runDialogSelectUi = {
  content: "z-50 max-h-72 overflow-y-auto",
};

const selectedRunCapabilityDetail = computed(() => {
  const capabilityID = selectedRunCapability.value?.value || "";
  if (!capabilityID) return "";
  const capability = capabilityOptions.value.find((item) => item.capabilityId === capabilityID);
  if (!capability?.description?.trim()) return "";
  return capability.description.trim();
});

watch(selectedRunCapabilityModule, (moduleOption) => {
  if (!isApprovalGuardedCapabilityWorkflow.value) return;
  const moduleKey = moduleOption?.value || "";
  const selectedCapabilityID = selectedRunCapability.value?.value || "";
  const selectedCapability = capabilityOptions.value.find((item) => item.capabilityId === selectedCapabilityID);
  if (selectedCapability && selectedCapability.module === moduleKey) return;
  selectedRunCapability.value = capabilitySelectItems.value[0] || null;
  approvalDebugForm.capability_id = selectedRunCapability.value?.value || "";
});

const humanReviewRolesText = computed(() => {
  const roles = selectedNode.value?.data?.props?.approver_policy?.roles;
  return Array.isArray(roles) ? roles.join(", ") : "";
});

const zoomPercent = computed(() => `${Math.round((viewport.value?.zoom || 1) * 100)}%`);
const workflowEditorStyle = computed(() => ({
  "--workflow-bottom-height": `${bottomPanelHeight.value}px`,
}));

const workflowDisplayName = computed(() => {
  const workflow = currentWorkflow.value;
  if (!workflow) return t("workflow.editor.untitled");
  const packNameKey = workflowPackI18nKey("name");
  if (packNameKey && te(packNameKey)) return t(packNameKey);
  return workflow.name || t("workflow.editor.untitled");
});
const currentWorkflowPackKey = computed(() => currentWorkflow.value?.raw?.workflow_pack_key?.trim() || "");
const isApprovalGuardedCapabilityWorkflow = computed(() => currentWorkflowPackKey.value === "approval_guarded_capability");

const latestRunState = computed(() => latestRun.value?.state || "");
const latestRunStateLabel = computed(() => latestRunState.value ? t(`workflow.state.${latestRunState.value}`) : t("workflow.editor.noRuns"));
const latestRunBadgeColor = computed(() => runStateColor(latestRunState.value));
const canCancelLatestRun = computed(() => {
  const state = latestRunState.value;
  return Boolean(latestRun.value?.uuid && ["queued", "running", "waiting", "compensating"].includes(state));
});
const workflowRuntimeConnectionLabel = computed(() => {
  if (workflowRuntimeBus.connected.value) return t("workflow.editor.runtimeConnected");
  if (workflowRuntimeBus.connecting.value) return t("workflow.editor.runtimeConnecting");
  return t("workflow.editor.runtimeDisconnected");
});
const workflowRuntimeConnectionColor = computed(() => {
  if (workflowRuntimeBus.connected.value) return "success";
  if (workflowRuntimeBus.connecting.value) return "warning";
  return "error";
});
const latestRunCardClass = computed(() => {
  const state = latestRunState.value;
  if (state === "succeeded") return "success";
  if (state === "failed" || state === "canceled") return "failed";
  if (state === "waiting") return "waiting";
  if (state === "queued" || state === "running" || state === "compensating") return "running";
  return "idle";
});

const latestRunSteps = computed(() => {
  const steps = latestRun.value?.steps || [];
  if (steps.length) {
    return steps.map((step) => {
      const state = normalizeEffectiveStepState(step.state);
      const startedAt = formatRunTimestamp(step.started_at || step.scheduled_at || "");
      const completedAt = formatRunTimestamp(step.completed_at || "");
      const error = step.error_message || step.failure_reason || step.error_code || "";
      return {
        id: step.step_id,
        label: stepDisplayName(step.step_id),
        kindLabel: stepKindLabel(step),
        stateLabel: t(`workflow.state.${state}`),
        stateClass: `state-${state}`,
        badgeColor: runStateColor(state),
        message: stepRunMessage(state, startedAt, completedAt, error),
      };
    });
  }
  const state = latestRun.value ? normalizeEffectiveStepState("pending") : "";
  return nodes.value.map((node) => ({
    id: node.id,
    label: displayLabel(node.data.label),
    kindLabel: node.data.kind ? t("workflow.editor.stepKindWithValue", { kind: getKindLabel(node.data.kind) }) : t("workflow.editor.unknownNodeKind"),
    stateLabel: state ? t(`workflow.state.${state}`) : t("workflow.editor.noRuns"),
    stateClass: state ? `state-${state}` : "",
    badgeColor: runStateColor(state),
    message: state === "skipped" ? t("workflow.editor.stepSkippedByCancel") : "",
  }));
});

const latestRunLogs = computed(() => {
  const steps = latestRun.value?.steps || [];
  return steps.map((step) => {
    const state = normalizeStepState(step.state || "queued");
    const startedAt = formatRunTimestamp(step.started_at || step.scheduled_at || "");
    const completedAt = formatRunTimestamp(step.completed_at || "");
    return {
      id: `${step.step_id}-${state}-${step.attempt || 0}`,
      stepID: step.step_id,
      title: stepDisplayName(step.step_id),
      kindLabel: stepKindLabel(step),
      message: t("workflow.editor.stepLogMessage", {
        state: t(`workflow.state.${state}`),
        started: startedAt || t("workflow.editor.notStarted"),
        completed: completedAt || t("workflow.editor.notCompleted"),
      }),
      error: step.error_message || step.failure_reason || step.error_code || "",
      stateClass: `state-${state}`,
    };
  });
});

// 过滤后的节点清单
const filteredPalette = computed(() => {
  if (!paletteSearch.value) return palette.value;

  const search = paletteSearch.value.toLowerCase();
  return palette.value.filter(
    (item) =>
      item.label.toLowerCase().includes(search) ||
      item.kind.toLowerCase().includes(search)
  );
});

const paletteCategoryOrder = [
  "input_output",
  "ai_skill",
  "platform_capability",
  "flow_control",
  "data_knowledge",
  "exception",
] as const;

const groupedPalette = computed(() => {
  const groups = new Map<string, typeof filteredPalette.value>();
  for (const item of filteredPalette.value) {
    const category = paletteCategoryForKind(item.kind);
    if (!groups.has(category)) groups.set(category, []);
    groups.get(category)!.push(item);
  }

  return paletteCategoryOrder
    .filter((key) => (groups.get(key) || []).length > 0)
    .map((key) => ({
      key,
      label: t(`workflow.editor.paletteCategory.${key}`),
      items: groups.get(key) || [],
    }));
});

const selectedNodeReviewTask = computed(() => {
  const stepID = selectedNode.value?.id || "";
  if (!stepID) return null;
  return latestReviewTasks.value.find((task) => task.step_id === stepID) || null;
});

const selectedNodeRunStep = computed(() => {
  const stepID = selectedNode.value?.id || "";
  if (!stepID) return null;
  return latestRun.value?.steps?.find((step) => step.step_id === stepID) || null;
});

const selectedNodeRunState = computed(() => normalizeStepState(selectedNodeRunStep.value?.state || selectedNode.value?.data?.runState || ""));
const selectedNodeRunStateLabel = computed(() => {
  if (!latestRun.value) return t("workflow.editor.noRuns");
  return t(`workflow.state.${selectedNodeRunState.value || "pending"}`);
});

// 获取节点类型标签
function getKindLabel(kind: string) {
  const label = kinds.value[kind]?.label;
  if (label && te(label)) return t(label);
  return kind;
}

function displayLabel(label: string) {
  return te(label) ? t(label) : label;
}

function schemaFieldLabel(key: string) {
  const i18nKey = `workflow.fields.${key}`;
  return te(i18nKey) ? t(i18nKey) : key;
}

function stepDisplayName(stepID: string) {
  const packKey = currentWorkflow.value?.raw?.workflow_pack_key?.trim();
  if (packKey) {
    const stepNameKey = `workflow.step.${camelCase(packKey)}.${camelCase(stepID)}.name`;
    if (te(stepNameKey)) return t(stepNameKey);
  }

  const node = nodes.value.find((item) => item.id === stepID);
  if (node?.data?.label) return displayLabel(node.data.label);
  return stepID;
}

function stepKindLabel(step: { node_kind?: string; type?: string }) {
  const kind = step.node_kind || step.type || "";
  if (!kind) return t("workflow.editor.unknownNodeKind");
  return t("workflow.editor.stepKindWithValue", { kind: getKindLabel(kind) });
}

function reviewTypeLabel(reviewType: string) {
  const key = `workflow.review.type.${camelCase(reviewType)}`;
  return te(key) ? t(key) : reviewType;
}

function paletteCategoryForKind(kind: string) {
  if (kind === "input.capture" || kind === "event.emit") return "input_output";
  if (kind === "skill.invoke" || kind === "llm.invoke") return "ai_skill";
  if (kind === "capability.invoke") return "platform_capability";
  if (kind === "human.review" || kind === "decision.gateway" || kind === "parallel.fanout" || kind === "parallel.join") return "flow_control";
  if (kind === "metadata.classify" || kind === "knowledge.stage" || kind === "knowledge.publish") return "data_knowledge";
  if (kind === "compensation.rollback") return "exception";
  return "flow_control";
}

function formatReviewPayload(payload?: Record<string, any>) {
  if (!payload || Object.keys(payload).length === 0) return t("workflow.editor.emptyPayload");
  return JSON.stringify(payload, null, 2);
}

function formatStepRunPayload(step: WorkflowStepRecord) {
  return JSON.stringify({
    state: normalizeStepState(step.state),
    payload_in: step.payload_in || {},
    payload_out: step.payload_out || {},
    error: step.error_message || step.failure_reason || step.error_code || "",
  }, null, 2);
}

function updateHumanReviewRoles(value: string) {
  if (!selectedNode.value) return;
  const roles = value
    .split(",")
    .map((role) => role.trim())
    .filter(Boolean);
  selectedNode.value.data.props.approver_policy = {
    ...(selectedNode.value.data.props.approver_policy || {}),
    roles,
  };
}

function isRuntimeTemplateValue(value: unknown) {
  return typeof value === "string" && /^\$\{[a-zA-Z0-9_]+\}$/.test(value.trim());
}

function normalizeSelectedNodeProps(node: Node) {
  node.data.props = node.data.props || {};
  if (node.data.kind === "human.review") {
    node.data.props.approver_policy = node.data.props.approver_policy || { roles: [] };
  }
  if (node.data.kind === "input.capture") {
    node.data.props.source_policy = node.data.props.source_policy || { text: false, form: false };
  }
}

async function copyToClipboard(value: string) {
  if (!import.meta.client || !navigator.clipboard) {
    toast.add({
      title: t("workflow.editor.copyUnavailable"),
      color: "error",
    });
    return;
  }
  await navigator.clipboard.writeText(value);
  toast.add({
    title: t("workflow.editor.copied"),
    color: "success",
  });
}

// 拖拽开始
function onDragStart(event: DragEvent, paletteId: string) {
  if (event.dataTransfer) {
    event.dataTransfer.setData("application/vueflow", paletteId);
    event.dataTransfer.effectAllowed = "move";
  }
}

// 拖拽放置
function onDrop(event: DragEvent) {
  if (!event.dataTransfer) return;

  const paletteId = event.dataTransfer.getData("application/vueflow");
  if (!paletteId) return;

  // 获取放置位置
  const position = project({
    x: event.clientX,
    y: event.clientY,
  });

  // 添加节点
  const newNode = addNodeFromPalette(paletteId, position);
  if (newNode) {
    addNodes([newNode]);
  }
}

// 适应视图
function handleFitView() {
  fitView();
}

function handleZoomIn() {
  void zoomIn({ duration: 140 });
}

function handleZoomOut() {
  void zoomOut({ duration: 140 });
}

function handleResetZoom() {
  void zoomTo(1, { duration: 140 });
}

function handleCenterView() {
  void setViewport(
    {
      x: 0,
      y: 0,
      zoom: viewport.value?.zoom || 1,
    },
    { duration: 140 }
  );
}

function startBottomResize(event: PointerEvent) {
  if (!import.meta.client) return;
  event.preventDefault();

  stopBottomResize?.();
  const startY = event.clientY;
  const startHeight = bottomPanelHeight.value;
  const minHeight = 160;
  const maxHeight = 520;

  const onPointerMove = (moveEvent: PointerEvent) => {
    const nextHeight = startHeight + startY - moveEvent.clientY;
    bottomPanelHeight.value = Math.min(maxHeight, Math.max(minHeight, nextHeight));
  };

  const onPointerUp = () => {
    stopBottomResize?.();
    stopBottomResize = null;
  };

  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp, { once: true });
  stopBottomResize = () => {
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
  };
}

// 保存工作流
async function handleSaveWorkflow() {
  await saveWorkflow(nodes.value, edges.value);
}

// 撤销/重做功能（简单实现）
const canUndo = ref(false);
const canRedo = ref(false);

function undo() {
  console.info("workflow.undo_requested");
}

function redo() {
  console.info("workflow.redo_requested");
}

// 允许拖拽
function onDragOver(event: DragEvent) {
  if (event.preventDefault) {
    event.preventDefault();
  }

  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = "move";
  }
}

// 连接节点
function handleConnect(params: Connection) {
  console.info("workflow.edge_connected", params);
  addEdges([params]);
}

// 节点拖拽结束
function onNodeDragStop() {
  // Vue Flow 会自动更新节点位置
}

// 画布准备完成
function onReady() {
  fitView();
}

// 点击节点
function onNodeClick(event: { node: Node }) {
  const node = event.node;
  const previousViewport = {
    x: viewport.value?.x || 0,
    y: viewport.value?.y || 0,
    zoom: viewport.value?.zoom || 1,
  };
  normalizeSelectedNodeProps(node);
  selectedNode.value = node;
  propertiesTab.value = latestReviewTasks.value.some((task) => task.step_id === node.id && task.status === "pending")
    ? "runtime"
    : "config";

  // 初始化对象属性编辑器
  if (node.data.props) {
    for (const [key, value] of Object.entries(node.data.props)) {
      if (typeof value === "object" && value !== null) {
        objectProps[key] = JSON.stringify(value, null, 2);
      }
    }
  }
  nextTick(() => {
    void setViewport(previousViewport, { duration: 0 });
  });
}

// 更新节点属性
function updateNodeProps(nodeId: string, newProps: Record<string, any>) {
  const node = findNode(nodeId);
  if (node && node.data) {
    node.data.props = { ...node.data.props, ...newProps };
  }
}

// 更新对象属性
function updateObjectProp(key: string) {
  if (!selectedNode.value) return;

  try {
    selectedNode.value.data.props[key] = JSON.parse(objectProps[key]);
  } catch (err) {
    console.error("workflow.property_json_invalid", { key, err });
    // 恢复原始值
    objectProps[key] = JSON.stringify(
      selectedNode.value.data.props[key],
      null,
      2
    );
  }
}

// 判断是否为枚举字段
function isEnumField(key: string, schema: any) {
  if (!schema?.properties?.[key]?.enum) return false;
  return Array.isArray(schema.properties[key].enum);
}

// 获取枚举选项
function getEnumOptions(key: string, schema: any) {
  if (!schema?.properties?.[key]?.enum) return [];
  return schema.properties[key].enum.map((value: any) => ({
    label: value,
    value,
  }));
}

// 获取数字输入步长
function getNumberStep(key: string, schema: any) {
  if (!schema?.properties?.[key]) return 1;

  const prop = schema.properties[key];
  if (prop.type !== "number") return 1;

  // 如果有最小和最大值，计算合适的步长
  if (prop.minimum !== undefined && prop.maximum !== undefined) {
    const range = prop.maximum - prop.minimum;
    if (range <= 1) return 0.1;
    if (range <= 10) return 0.5;
    if (range <= 100) return 1;
    return Math.pow(10, Math.floor(Math.log10(range)) - 2);
  }

  return 1;
}

function openRunDialog() {
  resetDebugInput();
  runError.value = "";
  runDialogOpen.value = true;
  void loadRunDialogReferenceData();
}

async function loadRunDialogReferenceData() {
  if (!isApprovalGuardedCapabilityWorkflow.value || capabilityOptions.value.length || capabilityOptionsLoading.value) return;
  capabilityOptionsLoading.value = true;
  capabilityOptionsError.value = "";
  try {
    const result = await PlatformCapabilityService.listModules();
    capabilityOptions.value = result.modules.flatMap((module) =>
      module.capabilities.map((capability) => ({
        ...capability,
        moduleDisplayName: module.displayName || module.module,
      }))
    );
    syncRunDialogSelects();
    if (!capabilityModuleSelectItems.value.length) {
      capabilityOptionsError.value = t("workflow.editor.capabilityOptionsEmpty");
      toast.add({
        title: t("workflow.editor.capabilityOptionsLoadFailed"),
        description: capabilityOptionsError.value,
        color: "error",
      });
    }
  } catch (err: any) {
    capabilityOptionsError.value = err?.message || t("workflow.editor.capabilityOptionsLoadFailed");
    toast.add({
      title: t("workflow.editor.capabilityOptionsLoadFailed"),
      description: capabilityOptionsError.value,
      color: "error",
    });
  } finally {
    capabilityOptionsLoading.value = false;
  }
}

function capabilityOptionLabel(capability: RunCapabilityOption) {
  const localizedKey = capabilityI18nKey(capability.capabilityId);
  return te(localizedKey) ? t(localizedKey) : readableCapabilityTitle(capability);
}

function capabilityModuleLabel(capability: RunCapabilityOption) {
  const moduleKey = `workflow.capabilityModule.${camelCase(capability.module || "")}`;
  if (te(moduleKey)) return t(moduleKey);
  const displayName = capability.moduleDisplayName?.trim() || "";
  if (displayName && displayName !== capability.module) return displayName;
  return humanizeModuleKey(capability.module || "");
}

function hasLocalizedCapabilityName(capability: RunCapabilityOption) {
  return te(capabilityI18nKey(capability.capabilityId));
}

function capabilityI18nKey(capabilityID: string) {
  return `workflow.capability.${camelCase(capabilityID.replace(/^com\.corex\./, "").replace(/[^a-zA-Z0-9]+/g, "_"))}`;
}

function readableCapabilityTitle(capability: RunCapabilityOption) {
  const title = capability.title?.trim() || "";
  if (title && !looksLikeTechnicalCapabilityTitle(title)) return title;
  return t("workflow.editor.unnamedCapability");
}

function hasReadableCapabilityTitle(capability: RunCapabilityOption) {
  const title = capability.title?.trim() || "";
  return Boolean(title && !looksLikeTechnicalCapabilityTitle(title));
}

function isWorkflowRunnableBusinessCapability(capability: RunCapabilityOption) {
  const capabilityID = capability.capabilityId || "";
  if (capabilityID.startsWith("com.corex.grpc.") || capabilityID.startsWith("com.corex.rest.")) return false;
  if (capabilityID.includes(".gin.")) return false;
  return hasLocalizedCapabilityName(capability) || hasReadableCapabilityTitle(capability);
}

function looksLikeTechnicalCapabilityTitle(title: string) {
  return /^(GET|POST|PUT|PATCH|DELETE)\s+\/api\//i.test(title) || title === title.toLowerCase() && title.includes(".");
}

// 运行工作流
async function runWorkflow() {
  const definitionUUID = currentWorkflow.value?.uuid;
  if (!definitionUUID) {
    runError.value = t("workflow.errors.noCurrentDefinition");
    return;
  }
  const debugInput = parseDebugInput();
  if (!debugInput) return;

  runLoading.value = true;
  runError.value = "";
  latestReviewTasks.value = [];
  clearCanvasRunState();
  try {
    const started = await workflowService.startInstance(definitionUUID, debugInput);
    applyWorkflowRuntimeInstance(started);
    bottomTab.value = "runs";
    runDialogOpen.value = false;
    toast.add({
      title: t("workflow.editor.runStarted"),
      description: t("workflow.editor.instanceShort", { uuid: shortUUID(started.uuid) }),
      color: "success",
    });
  } catch (err: any) {
    const message = err?.message || t("workflow.editor.runFailed");
    runError.value = message;
    bottomTab.value = "runs";
    toast.add({
      title: t("workflow.editor.runFailed"),
      description: message,
      color: "error",
    });
  } finally {
    runLoading.value = false;
  }
}

function parseDebugInput() {
  try {
    if (isApprovalGuardedCapabilityWorkflow.value) {
      return buildApprovalDebugInputFromForm();
    }
    const parsed = JSON.parse(debugInputText.value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error(t("workflow.editor.debugInputObjectRequired"));
    }
    return parsed as Record<string, any>;
  } catch (err: any) {
    const message = t("workflow.editor.debugInputInvalid");
    runError.value = message;
    bottomTab.value = "runs";
    toast.add({
      title: t("workflow.editor.debugInputInvalid"),
      description: err?.message || t("workflow.editor.debugInputInvalidDescription"),
      color: "error",
    });
    return null;
  }
}

function resetDebugInput() {
  const input = buildDebugInputForCurrentWorkflow();
  if (isApprovalGuardedCapabilityWorkflow.value) {
    approvalDebugForm.capability_id = String(input.capability_id || "");
    approvalDebugForm.reason = String(input.request?.reason || "");
    approvalDebugForm.dry_run = Boolean(input.request?.payload?.dry_run);
    approvalDebugForm.note = String(input.request?.payload?.note || "");
    syncRunDialogSelects();
  }
  debugInputText.value = JSON.stringify(input, null, 2);
}

function syncRunDialogSelects() {
  const selectedCapability = workflowRunnableCapabilities.value.find(
    (item) => item.capabilityId === approvalDebugForm.capability_id
  );
  selectedRunCapabilityModule.value =
    capabilityModuleSelectItems.value.find((item) => item.value === selectedCapability?.module) ||
    capabilityModuleSelectItems.value[0] ||
    null;
  selectedRunCapability.value = selectedCapability
    ? capabilitySelectItems.value.find((item) => item.value === selectedCapability.capabilityId) || null
    : capabilitySelectItems.value[0] || null;
  approvalDebugForm.capability_id = selectedRunCapability.value?.value || approvalDebugForm.capability_id;
  selectedExecutionReason.value =
    executionReasonOptions.value.find((item) => item.value === approvalDebugForm.reason) || executionReasonOptions.value[0] || null;
}

async function restoreLatestRun(definitionUUID: string) {
  runError.value = "";
  latestReviewTasks.value = [];

  const result = await workflowService.listInstances({
    page: 1,
    pageSize: 1,
    definition_uuid: definitionUUID,
    include_steps: true,
  });
  const latest = result.items?.[0];
  if (!latest) {
    latestRun.value = null;
    return;
  }

  latestRun.value = latest;
  applyRunStateToCanvas();
  await refreshReviewTasksForRun(latest);
}

async function refreshReviewTasksForRun(run: WorkflowInstance | null) {
  if (!run || run.state !== "waiting") {
    latestReviewTasks.value = [];
    return;
  }
  const reviews = await workflowService.listReviewTasks({
    page: 1,
    pageSize: 20,
    status: "pending",
    workflow_instance_uuid: run.uuid,
  });
  latestReviewTasks.value = reviews.items || [];
  if (latestReviewTasks.value.length > 0) {
    focusRuntimeStep(latestReviewTasks.value[0].step_id);
  }
}

async function actReviewTask(task: HumanReviewTask, action: "approve" | "reject") {
  actingReviewTaskUUID.value = task.review_task_uuid;
  actingReviewAction.value = action;
  try {
    const reviewPayload = task.payload && typeof task.payload === "object" && !Array.isArray(task.payload)
      ? { ...task.payload }
      : {};
    await workflowService.actReviewTask(task.review_task_uuid, {
      action,
      payload: {
        ...reviewPayload,
        review: {
          workflow_instance_uuid: task.workflow_instance_uuid,
          step_id: task.step_id,
          action,
        },
        workflow_instance_uuid: task.workflow_instance_uuid,
        step_id: task.step_id,
      },
    });
    toast.add({
      title: action === "approve" ? t("workflow.review.approveSuccess") : t("workflow.review.rejectSuccess"),
      color: action === "approve" ? "success" : "warning",
    });
    await restoreLatestRun(currentWorkflow.value?.uuid || "");
  } catch (err: any) {
    toast.add({
      title: t("workflow.review.actionFailed"),
      description: err?.message || t("workflow.review.actionFailed"),
      color: "error",
    });
  } finally {
    actingReviewTaskUUID.value = "";
    actingReviewAction.value = "";
  }
}

async function cancelLatestRun() {
  const instanceUUID = latestRun.value?.uuid || "";
  if (!instanceUUID || !canCancelLatestRun.value) return;
  cancelRunLoading.value = true;
  try {
    const updated = await workflowService.controlInstance(instanceUUID, {
      action: "cancel",
      reason: "workflow_debug_run_canceled",
    });
    applyWorkflowRuntimeInstance(updated);
    latestReviewTasks.value = [];
    toast.add({
      title: t("workflow.editor.cancelRunSuccess"),
      color: "warning",
    });
  } catch (err: any) {
    toast.add({
      title: t("workflow.editor.cancelRunFailed"),
      description: err?.message || t("workflow.editor.cancelRunFailed"),
      color: "error",
    });
  } finally {
    cancelRunLoading.value = false;
  }
}

function normalizeStepState(state?: string) {
  if (!state) return "pending";
  if (state === "completed") return "succeeded";
  if (state === "in_progress") return "running";
  return state;
}

function normalizeEffectiveStepState(state?: string) {
  const normalized = normalizeStepState(state);
  if (latestRunState.value !== "canceled") return normalized;
  if (["succeeded", "failed", "canceled", "approved", "rejected"].includes(normalized)) return normalized;
  return "skipped";
}

function stepRunMessage(state: string, startedAt: string, completedAt: string, error: string) {
  if (error) return error;
  if (state === "skipped") return t("workflow.editor.stepSkippedByCancel");
  if (completedAt) return t("workflow.editor.stepCompletedAt", { time: completedAt });
  if (startedAt) return t("workflow.editor.stepStartedAt", { time: startedAt });
  return t("workflow.editor.notStarted");
}

function handleWorkflowRuntimeEvent(event: WorkflowRuntimeEvent) {
  applyWorkflowRuntimeInstance(event.instance);
}

function applyWorkflowRuntimeInstance(instance: WorkflowInstance) {
  latestRun.value = instance;
  applyRunStateToCanvas();
  void refreshReviewTasksForRun(instance);
  if (instance.state === "waiting" && instance.current_step_id) {
    focusRuntimeStep(instance.current_step_id);
  }
}

function applyRunStateToCanvas() {
  const stepStates = new Map<string, string>();
  for (const step of latestRun.value?.steps || []) {
    stepStates.set(step.step_id, normalizeStepState(step.state));
  }

  nodes.value = nodes.value.map((node) => ({
    ...node,
    data: {
      ...node.data,
      runState: latestRun.value ? stepStates.get(node.id) || "pending" : "",
    },
  }));

  edges.value = edges.value.map((edge) => {
    const runState = edgeRunState(stepStates.get(edge.source), stepStates.get(edge.target));
    return {
      ...edge,
      animated: runState === "active",
      class: runState ? `workflow-run-edge workflow-run-edge-${runState}` : "",
      data: {
        ...(edge.data || {}),
        runState,
      },
    };
  });
}

function edgeRunState(sourceState?: string, targetState?: string) {
  if (!latestRun.value) return "";
  if (!sourceState) return "pending";
  if (!targetState) return isFailureState(sourceState) ? "blocked" : "pending";
  if (isFailureState(targetState)) return "failed";
  if (targetState === "running" || targetState === "queued" || targetState === "waiting" || targetState === "compensating") return "active";
  if (targetState === "succeeded" || targetState === "completed") return "passed";
  return "pending";
}

function isFailureState(state?: string) {
  return state === "failed" || state === "canceled";
}

function clearCanvasRunState() {
  latestRun.value = null;
  nodes.value = nodes.value.map((node) => ({
    ...node,
    data: {
      ...node.data,
      runState: "",
    },
  }));
  edges.value = edges.value.map((edge) => ({
    ...edge,
    animated: false,
    class: "",
    data: {
      ...(edge.data || {}),
      runState: "",
    },
  }));
}

function selectRunStep(stepID: string) {
  if (!stepID) return;
  const node = findNode(stepID);
  if (!node) {
    void copyToClipboard(stepID);
    return;
  }
  normalizeSelectedNodeProps(node);
  selectedNode.value = node;
  propertiesTab.value = "runtime";
}

function focusRuntimeStep(stepID: string) {
  if (!stepID) return;
  const node = findNode(stepID);
  if (!node) return;
  normalizeSelectedNodeProps(node);
  selectedNode.value = node;
  propertiesTab.value = "runtime";
}

function toggleTheme() {
  colorMode.preference = isDark.value ? "light" : "dark";
}

function toggleFullscreen() {
  if (!import.meta.client) return;

  if (!document.fullscreenElement) {
    void document.documentElement.requestFullscreen?.();
    return;
  }

  void document.exitFullscreen?.();
}

function syncWorkflowToCanvas() {
  const workflow = currentWorkflow.value;
  if (!workflow) return;
  nodes.value = workflow.nodes.map(workflowNodeToVueFlowNode);
  edges.value = workflow.edges.map(workflowEdgeToVueFlowEdge);
  selectedNode.value = null;
  hasLoadedCanvas.value = true;
  applyRunStateToCanvas();
  nextTick(() => fitView());
}

function workflowNodeToVueFlowNode(node: WfNode): Node {
  return {
    id: node.id,
    type: "generic",
    position: node.position,
    data: {
      kind: node.kind,
      paletteId: node.paletteId,
      label: node.label,
      props: node.props || {},
      ui: node.ui,
      ports: kinds.value[node.kind]?.ports || {
        inputs: [{ name: "in" }],
        outputs: [{ name: "out" }, { name: "error" }],
      },
      schema: kinds.value[node.kind]?.schema || { type: "object", properties: {} },
    },
    width: node.ui.size?.w,
    height: node.ui.size?.h,
  };
}

function workflowEdgeToVueFlowEdge(edge: WorkflowEdge): Edge {
  return {
    id: edge.id,
    source: edge.source,
    sourceHandle: edge.sourceHandle || "out",
    target: edge.target,
    targetHandle: edge.targetHandle || "in",
    label: edge.label,
    type: edge.type || "smoothstep",
    animated: false,
  };
}

function workflowPackI18nKey(field: "name" | "description") {
  const packKey = currentWorkflow.value?.raw?.workflow_pack_key?.trim();
  if (!packKey) return "";
  return `workflow.pack.${camelCase(packKey)}.${field}`;
}

function buildDebugInputForCurrentWorkflow() {
  const packKey = currentWorkflow.value?.raw?.workflow_pack_key?.trim();
  if (packKey === "approval_guarded_capability") {
    return buildApprovalDebugInputFromForm(false);
  }
  if (packKey === "intake_classify_review") {
    return {
      taxonomy_namespace: "corex.marketing.methodology",
      tag_namespace: "corex.marketing",
      dictionary_namespace: "corex.marketing",
      resource_type_namespace: "corex.knowledge",
      intake: {
        text: "workflow_debug_metadata_intake",
        source: "workflow_editor_run_test",
      },
    };
  }
  if (packKey === "skill_review_publish_event") {
    return {
      skill_id: "debug.echo",
      input: {
        text: "workflow_debug_skill_review",
      },
    };
  }
  return {};
}

function buildApprovalDebugInputFromForm(validate = true) {
  if (validate || selectedRunCapability.value) {
    approvalDebugForm.capability_id = selectedRunCapability.value?.value || "";
  }
  if (validate || selectedExecutionReason.value) {
    approvalDebugForm.reason = selectedExecutionReason.value?.value || "";
  }
  if (validate) {
    if (capabilityOptionsLoading.value) {
      throw new Error(t("workflow.editor.capabilityOptionsLoading"));
    }
    if (!selectedRunCapabilityModule.value?.value) {
      throw new Error(t("workflow.editor.debugCapabilityModuleRequired"));
    }
    if (!approvalDebugForm.capability_id.trim()) {
      throw new Error(t("workflow.editor.debugCapabilityRequired"));
    }
    if (!capabilitySelectItems.value.some((item) => item.value === approvalDebugForm.capability_id.trim())) {
      throw new Error(t("workflow.editor.debugCapabilityMustSelect"));
    }
    if (!executionReasonOptions.value.some((item) => item.value === approvalDebugForm.reason.trim())) {
      throw new Error(t("workflow.editor.debugReasonMustSelect"));
    }
  }
  return {
    capability_id: approvalDebugForm.capability_id.trim(),
    request: {
      reason: approvalDebugForm.reason.trim(),
      payload: {
        dry_run: approvalDebugForm.dry_run,
        note: approvalDebugForm.note.trim(),
      },
    },
  };
}

function camelCase(value: string) {
  return value
    .trim()
    .replace(/[^a-zA-Z0-9]+(.)/g, (_, char: string) => char.toUpperCase())
    .replace(/^[A-Z]/, (char) => char.toLowerCase());
}

function humanizeModuleKey(value: string) {
  return value
    .trim()
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function runStateColor(state: string) {
  if (state === "succeeded") return "success";
  if (state === "failed" || state === "canceled") return "error";
  if (state === "waiting") return "warning";
  if (state === "skipped") return "neutral";
  if (!state) return "neutral";
  return "primary";
}

function shortUUID(value: string) {
  return value.length > 8 ? value.slice(0, 8) : value;
}

function formatRunTimestamp(value?: string | null) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString();
}

watch(currentWorkflow, () => {
  if (!hasLoadedCanvas.value) {
    syncWorkflowToCanvas();
  }
});

watch(latestRun, () => {
  applyRunStateToCanvas();
});

onMounted(async () => {
  unsubscribeWorkflowRuntime = workflowRuntimeBus.subscribe(handleWorkflowRuntimeEvent, {
    definitionUUID: () => currentWorkflow.value?.uuid || "",
    instanceUUID: () => latestRun.value?.uuid || "",
  });
  await loadNodeCatalog();
  const workflowId = typeof route.query.id === "string" ? route.query.id : "";
  if (workflowId) {
    await loadWorkflow(workflowId);
    syncWorkflowToCanvas();
    resetDebugInput();
    await restoreLatestRun(workflowId);
  }
});

onBeforeUnmount(() => {
  stopBottomResize?.();
  stopBottomResize = null;
  unsubscribeWorkflowRuntime?.();
  unsubscribeWorkflowRuntime = null;
});
</script>

<style scoped>
.workflow-editor {
  --wf-bg: #f6f8fc;
  --wf-panel: #ffffff;
  --wf-panel-soft: #f8fafc;
  --wf-border: #e5e9f2;
  --wf-text: #111827;
  --wf-muted: #667085;
  --wf-dim: #98a2b3;
  --wf-accent: #2563eb;
  --wf-canvas: #fbfcff;
  --wf-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);
  display: grid;
  grid-template-rows: 72px 34px minmax(0, 1fr) var(--workflow-bottom-height, 260px);
  height: 100vh;
  width: 100%;
  overflow: hidden;
  background: var(--wf-bg);
  color: var(--wf-text);
}

.workflow-editor.dark {
  --wf-bg: #0b1220;
  --wf-panel: #111827;
  --wf-panel-soft: #172033;
  --wf-border: rgba(148, 163, 184, 0.22);
  --wf-text: #f8fafc;
  --wf-muted: #cbd5e1;
  --wf-dim: #94a3b8;
  --wf-accent: #60a5fa;
  --wf-canvas: #0f172a;
  --wf-shadow: 0 16px 38px rgba(0, 0, 0, 0.28);
}

.workflow-topbar {
  display: grid;
  grid-template-columns: minmax(320px, 1fr) minmax(320px, 520px) minmax(360px, 1fr);
  align-items: center;
  gap: 20px;
  padding: 12px 20px;
  border-bottom: 1px solid var(--wf-border);
  background: color-mix(in srgb, var(--wf-panel) 94%, transparent);
}

.workflow-title-group,
.workflow-top-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.workflow-top-actions {
  justify-content: flex-end;
}

.workflow-title {
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 20px;
  font-weight: 700;
  color: var(--wf-text);
}

.workflow-subtitle {
  display: flex;
  gap: 16px;
  margin-top: 4px;
  color: var(--wf-muted);
  font-size: 12px;
}

.workflow-global-search {
  width: 100%;
}

.workflow-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  min-height: 0;
  border-bottom: 1px solid var(--wf-border);
  background: var(--wf-panel);
}

.metric-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 0 16px;
  border-right: 1px solid var(--wf-border);
}

.metric-cell span {
  flex: none;
  color: var(--wf-dim);
  font-size: 12px;
}

.metric-cell strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wf-text);
  font-size: 14px;
  font-weight: 700;
}

.workflow-main {
  display: grid;
  grid-template-columns: 248px minmax(0, 1fr) 360px;
  min-height: 0;
  overflow: hidden;
}

.workflow-palette,
.workflow-properties,
.workflow-bottom-panel {
  background: var(--wf-panel);
}

.workflow-palette {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  border-right: 1px solid var(--wf-border);
}

.palette-header {
  display: flex;
  align-items: center;
  height: 52px;
  padding: 0 16px;
  border-bottom: 1px solid var(--wf-border);
}

.palette-header h3,
.run-list h3,
.run-logs h3 {
  font-size: 14px;
  font-weight: 700;
  color: var(--wf-text);
}

.palette-search {
  padding: 12px 16px;
}

.palette-items {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 12px 14px;
}

.palette-group {
  display: grid;
  gap: 8px;
  margin-bottom: 16px;
}

.palette-group-title {
  position: sticky;
  z-index: 1;
  top: 0;
  padding: 6px 2px 4px;
  background: var(--wf-panel);
  color: var(--wf-dim);
  font-size: 11px;
  font-weight: 800;
}

.palette-item {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 52px;
  margin-bottom: 0;
  padding: 9px 12px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  cursor: grab;
  color: var(--wf-text);
}

.palette-item:hover {
  border-color: color-mix(in srgb, var(--wf-accent) 45%, var(--wf-border));
  box-shadow: 0 6px 18px rgba(37, 99, 235, 0.12);
}

.palette-item-icon,
.node-avatar {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  flex: none;
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-accent) 12%, var(--wf-panel));
  color: var(--wf-accent);
}

.palette-icon-input_output {
  background: color-mix(in srgb, #14b8a6 16%, var(--wf-panel));
  color: #2dd4bf;
}

.palette-icon-ai_skill {
  background: color-mix(in srgb, #8b5cf6 16%, var(--wf-panel));
  color: #a78bfa;
}

.palette-icon-platform_capability {
  background: color-mix(in srgb, #2563eb 16%, var(--wf-panel));
  color: #60a5fa;
}

.palette-icon-flow_control {
  background: color-mix(in srgb, #f59e0b 17%, var(--wf-panel));
  color: #fbbf24;
}

.palette-icon-data_knowledge {
  background: color-mix(in srgb, #22c55e 16%, var(--wf-panel));
  color: #4ade80;
}

.palette-icon-exception {
  background: color-mix(in srgb, #ef4444 16%, var(--wf-panel));
  color: #f87171;
}

.palette-item-content {
  min-width: 0;
}

.palette-item-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 700;
}

.palette-item-kind {
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wf-muted);
  font-size: 12px;
}

.workflow-canvas-shell {
  position: relative;
  min-width: 0;
  overflow: hidden;
  background: var(--wf-canvas);
}

.canvas-toolbar {
  position: absolute;
  z-index: 6;
  top: 12px;
  left: 50%;
  display: flex;
  align-items: center;
  gap: 5px;
  height: 38px;
  padding: 4px 7px;
  border: 1px solid var(--wf-border);
  border-radius: 9px;
  background: color-mix(in srgb, var(--wf-panel) 96%, transparent);
  box-shadow: var(--wf-shadow);
  transform: translateX(-50%);
}

.canvas-tool-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  flex: none;
  border-radius: 7px;
  color: var(--wf-muted);
}

.canvas-tool-button:hover:not(:disabled),
.canvas-tool-button.active {
  background: var(--wf-panel-soft);
  color: var(--wf-accent);
}

.canvas-tool-button:disabled {
  cursor: not-allowed;
  color: color-mix(in srgb, var(--wf-dim) 45%, transparent);
}

.canvas-tool-button :deep(svg) {
  width: 16px;
  height: 16px;
}

.canvas-toolbar-separator {
  width: 1px;
  height: 18px;
  margin: 0 2px;
  background: var(--wf-border);
}

.zoom-chip {
  height: 26px;
  min-width: 56px;
  border: 1px solid var(--wf-border);
  border-radius: 7px;
  padding: 0 10px;
  background: color-mix(in srgb, var(--wf-panel-soft) 78%, var(--wf-panel));
  color: var(--wf-text);
  font-size: 12px;
  font-weight: 700;
  user-select: none;
}

.zoom-chip:hover {
  border-color: color-mix(in srgb, var(--wf-accent) 38%, var(--wf-border));
  color: var(--wf-accent);
}

.workflow-vue-flow {
  width: 100%;
  height: 100%;
}

.workflow-properties {
  display: flex;
  flex-direction: column;
  min-width: 0;
  border-left: 1px solid var(--wf-border);
}

.properties-tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  border-bottom: 1px solid var(--wf-border);
}

.properties-tab {
  height: 48px;
  border-bottom: 2px solid transparent;
  color: var(--wf-muted);
  font-size: 13px;
  font-weight: 700;
}

.properties-tab.active {
  border-bottom-color: var(--wf-accent);
  color: var(--wf-accent);
}

.properties-content {
  flex: 1;
  overflow-y: auto;
  padding: 18px;
}

.properties-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.properties-header h4 {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  font-weight: 700;
  color: var(--wf-text);
}

.properties-kind,
.properties-empty,
.run-logs p {
  color: var(--wf-muted);
  font-size: 12px;
}

.properties-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 18px;
}

.properties-switch-grid {
  display: grid;
  gap: 10px;
}

.properties-switch-grid label {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--wf-text);
  font-size: 13px;
}

.properties-note {
  border: 1px solid color-mix(in srgb, var(--wf-accent) 32%, var(--wf-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-accent) 8%, var(--wf-panel));
  padding: 9px 10px;
  color: var(--wf-muted);
  font-size: 12px;
  line-height: 1.5;
}

.node-runtime-panel {
  display: grid;
  gap: 14px;
}

.node-runtime-summary,
.runtime-review-card,
.runtime-payload-card,
.runtime-empty-card {
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 12px;
}

.node-runtime-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.node-runtime-summary span,
.runtime-review-card span,
.runtime-payload-card span {
  color: var(--wf-muted);
  font-size: 12px;
}

.node-runtime-summary strong,
.runtime-review-card strong {
  color: var(--wf-text);
  font-size: 14px;
}

.node-runtime-summary.state-running,
.node-runtime-summary.state-queued,
.node-runtime-summary.state-waiting {
  border-color: color-mix(in srgb, #f59e0b 45%, var(--wf-border));
  background: color-mix(in srgb, #f59e0b 8%, var(--wf-panel));
}

.node-runtime-summary.state-succeeded {
  border-color: color-mix(in srgb, #22c55e 42%, var(--wf-border));
  background: color-mix(in srgb, #22c55e 8%, var(--wf-panel));
}

.node-runtime-summary.state-failed,
.node-runtime-summary.state-canceled {
  border-color: color-mix(in srgb, #ef4444 42%, var(--wf-border));
  background: color-mix(in srgb, #ef4444 8%, var(--wf-panel));
}

.runtime-review-card {
  display: grid;
  gap: 12px;
  border-color: color-mix(in srgb, #f59e0b 45%, var(--wf-border));
  background: color-mix(in srgb, #f59e0b 8%, var(--wf-panel));
}

.runtime-review-card > div:first-child {
  display: grid;
  gap: 4px;
}

.runtime-review-card small {
  color: var(--wf-dim);
  font-size: 12px;
}

.runtime-review-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.runtime-payload-card {
  display: grid;
  gap: 8px;
}

.runtime-payload-card pre {
  max-height: 220px;
  overflow: auto;
  border-radius: 6px;
  background: color-mix(in srgb, #020617 72%, var(--wf-panel));
  padding: 10px;
  color: #dbeafe;
  font-size: 11px;
  line-height: 1.45;
  white-space: pre-wrap;
}

.runtime-empty-card {
  color: var(--wf-muted);
  font-size: 13px;
  line-height: 1.5;
}

.properties-empty {
  display: flex;
  min-height: 220px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  text-align: center;
}

.properties-empty :deep(svg) {
  width: 28px;
  height: 28px;
}

.node-help-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 18px;
}

.node-help-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.node-help-block > span {
  color: var(--wf-dim);
  font-size: 12px;
  font-weight: 700;
}

.node-help-block > strong {
  color: var(--wf-text);
  font-size: 13px;
  word-break: break-word;
}

.help-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.help-tags span {
  max-width: 100%;
  overflow-wrap: anywhere;
  border-radius: 999px;
  background: var(--wf-panel-soft);
  padding: 4px 8px;
  color: var(--wf-muted);
  font-size: 12px;
}

.workflow-bottom-panel {
  position: relative;
  display: grid;
  grid-template-rows: 44px minmax(0, 1fr);
  min-height: 0;
  border-top: 1px solid var(--wf-border);
}

.bottom-resize-handle {
  position: absolute;
  z-index: 10;
  top: -4px;
  left: 0;
  right: 0;
  height: 8px;
  cursor: ns-resize;
  touch-action: none;
}

.bottom-resize-handle::after {
  position: absolute;
  top: 3px;
  left: 50%;
  width: 72px;
  height: 2px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--wf-dim) 42%, transparent);
  content: "";
  transform: translateX(-50%);
}

.bottom-resize-handle:hover::after {
  background: color-mix(in srgb, var(--wf-accent) 64%, transparent);
}

.bottom-tabs {
  display: flex;
  align-items: center;
  gap: 28px;
  min-width: 0;
  padding: 0 20px;
  overflow-x: auto;
  border-bottom: 1px solid var(--wf-border);
}

.bottom-tab {
  height: 44px;
  flex: none;
  border-bottom: 2px solid transparent;
  color: var(--wf-muted);
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
}

.bottom-tab.active {
  border-bottom-color: var(--wf-accent);
  color: var(--wf-accent);
}

.bottom-content {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  min-height: 0;
  overflow: hidden;
}

.run-list,
.run-detail,
.run-logs {
  min-width: 0;
  min-height: 0;
  padding: 16px;
  border-right: 1px solid var(--wf-border);
  overflow: auto;
}

.run-logs {
  border-right: 0;
}

.run-card {
  margin-top: 12px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  padding: 12px;
  background: var(--wf-panel-soft);
}

.run-card span,
.run-card strong {
  display: block;
  font-size: 12px;
}

.run-card strong {
  margin-top: 4px;
  color: var(--wf-text);
}

.run-card small {
  display: block;
  margin-top: 6px;
  color: var(--wf-muted);
  font-size: 11px;
}

.run-card.success {
  border-color: color-mix(in srgb, #22c55e 38%, var(--wf-border));
  background: color-mix(in srgb, #22c55e 10%, var(--wf-panel));
}

.run-card.success strong {
  color: #16a34a;
}

.run-card.failed {
  border-color: color-mix(in srgb, #ef4444 42%, var(--wf-border));
  background: color-mix(in srgb, #ef4444 9%, var(--wf-panel));
}

.run-card.failed strong {
  color: #dc2626;
}

.run-card.waiting {
  border-color: color-mix(in srgb, #f59e0b 42%, var(--wf-border));
  background: color-mix(in srgb, #f59e0b 10%, var(--wf-panel));
}

.run-card.waiting strong {
  color: #d97706;
}

.run-card.running {
  border-color: color-mix(in srgb, var(--wf-accent) 45%, var(--wf-border));
  background: color-mix(in srgb, var(--wf-accent) 10%, var(--wf-panel));
}

.run-card.running strong {
  color: var(--wf-accent);
}

.run-detail-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.run-meta,
.run-error {
  margin-bottom: 10px;
  font-size: 12px;
}

.run-meta {
  color: var(--wf-muted);
}

.run-dialog-body {
  display: grid;
  gap: 18px;
}

.run-dialog-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-panel-soft) 72%, var(--wf-panel));
  padding: 14px 16px;
}

.run-dialog-summary div {
  display: grid;
  gap: 3px;
}

.run-dialog-summary span {
  color: var(--wf-muted);
  font-size: 12px;
}

.run-dialog-summary strong {
  color: var(--wf-text);
  font-size: 14px;
}

.run-dialog-status {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.run-dialog-footer {
  display: flex;
  width: 100%;
  justify-content: flex-end;
  gap: 8px;
}

.debug-input-panel {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-panel-soft) 72%, var(--wf-panel));
  padding: 10px;
}

.debug-input-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.debug-input-header strong {
  color: var(--wf-text);
  font-size: 13px;
}

.debug-input-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px 20px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-panel-soft) 76%, var(--wf-panel));
  padding: 18px;
}

.run-dialog-notice {
  grid-column: 1 / -1;
}

.debug-input-switch {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  color: var(--wf-text);
  font-size: 13px;
}

.debug-input-switch span {
  display: grid;
  gap: 3px;
}

.debug-input-switch strong {
  color: var(--wf-text);
  font-size: 13px;
}

.debug-input-switch small {
  color: var(--wf-muted);
  font-size: 12px;
  line-height: 1.45;
}

.run-field-hint {
  margin-top: 6px;
  color: var(--wf-muted);
  font-size: 12px;
  line-height: 1.5;
}

.run-capability-detail {
  margin-top: 8px;
  border-left: 2px solid color-mix(in srgb, var(--wf-accent) 48%, var(--wf-border));
  padding-left: 10px;
  color: var(--wf-dim);
  font-size: 12px;
  line-height: 1.5;
}

.run-dialog-error {
  grid-column: 1 / -1;
  border: 1px solid color-mix(in srgb, #ef4444 38%, var(--wf-border));
  border-radius: 8px;
  background: color-mix(in srgb, #ef4444 8%, var(--wf-panel));
  padding: 9px 10px;
  color: #dc2626;
  font-size: 12px;
}

.debug-input-textarea :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 12px;
}

.review-task-list {
  display: grid;
  gap: 8px;
  margin-bottom: 12px;
}

.review-task-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border: 1px solid color-mix(in srgb, #f59e0b 35%, var(--wf-border));
  border-radius: 8px;
  background: color-mix(in srgb, #f59e0b 8%, var(--wf-panel));
  padding: 8px 10px;
}

.review-task-row > div:first-child {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.review-task-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
}

.review-task-row strong {
  color: var(--wf-text);
  font-size: 12px;
}

.review-task-row span {
  color: var(--wf-muted);
  font-size: 11px;
}

.review-task-row code,
.run-log-id code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wf-dim);
  font-size: 11px;
}

.run-error {
  border: 1px solid color-mix(in srgb, #ef4444 36%, var(--wf-border));
  border-radius: 8px;
  padding: 8px 10px;
  color: #dc2626;
  background: color-mix(in srgb, #ef4444 8%, var(--wf-panel));
}

.run-step-list {
  display: grid;
  gap: 8px;
}

.run-step-row {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 9px 10px;
}

.run-step-marker {
  width: 10px;
  height: 10px;
  justify-self: center;
  border-radius: 999px;
  background: var(--wf-dim);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--wf-dim) 10%, transparent);
}

.run-step-main {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.run-step-main strong,
.run-step-main small,
.run-step-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-step-main strong {
  color: var(--wf-text);
  font-size: 12px;
}

.run-step-main small,
.run-step-main span {
  color: var(--wf-muted);
  font-size: 11px;
}

.run-step-main span {
  color: var(--wf-dim);
}

.run-step-status {
  justify-self: end;
  white-space: nowrap;
}

.run-step-action {
  justify-self: end;
}

.run-step-row.state-succeeded .run-step-marker {
  background: #22c55e;
  box-shadow: 0 0 0 4px color-mix(in srgb, #22c55e 13%, transparent);
}

.run-step-row.state-failed .run-step-marker,
.run-step-row.state-canceled .run-step-marker {
  background: #ef4444;
  box-shadow: 0 0 0 4px color-mix(in srgb, #ef4444 13%, transparent);
}

.run-step-row.state-waiting .run-step-marker,
.run-step-row.state-pending .run-step-marker {
  background: #f59e0b;
  box-shadow: 0 0 0 4px color-mix(in srgb, #f59e0b 13%, transparent);
}

.run-step-row.state-running .run-step-marker,
.run-step-row.state-queued .run-step-marker,
.run-step-row.state-compensating .run-step-marker {
  background: var(--wf-accent);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--wf-accent) 13%, transparent);
}

.run-step-row.state-skipped .run-step-marker {
  background: var(--wf-muted);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--wf-muted) 12%, transparent);
}

.run-log-list {
  display: grid;
  gap: 8px;
}

.run-log-row {
  display: grid;
  gap: 6px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 9px 10px;
}

.run-log-row.state-completed,
.run-log-row.state-succeeded {
  border-color: color-mix(in srgb, #22c55e 30%, var(--wf-border));
}

.run-log-row.state-waiting {
  border-color: color-mix(in srgb, #f59e0b 38%, var(--wf-border));
}

.run-log-row.state-failed {
  border-color: color-mix(in srgb, #ef4444 38%, var(--wf-border));
}

.run-log-main {
  display: grid;
  gap: 3px;
}

.run-log-main strong {
  color: var(--wf-text);
  font-size: 12px;
}

.run-log-main span {
  color: var(--wf-muted);
  font-size: 11px;
}

.run-log-main small {
  color: var(--wf-dim);
  font-size: 11px;
}

.run-log-id {
  display: inline-flex;
  align-items: center;
  max-width: 240px;
  gap: 4px;
  border: 0;
  border-radius: 6px;
  background: color-mix(in srgb, var(--wf-panel) 74%, var(--wf-panel-soft));
  padding: 2px 2px 2px 8px;
  cursor: pointer;
  text-align: left;
}

.run-log-id:hover {
  background: color-mix(in srgb, var(--wf-accent) 12%, var(--wf-panel));
}

.run-log-row > code {
  white-space: pre-wrap;
  color: #dc2626;
  font-size: 11px;
}

:deep(.vue-flow__minimap) {
  z-index: 8;
  left: 18px;
  bottom: 16px;
  margin: 0;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel) !important;
  box-shadow: var(--wf-shadow);
}

:deep(.workflow-run-edge .vue-flow__edge-path) {
  stroke-width: 2.5;
  transition: stroke 160ms ease, stroke-width 160ms ease, opacity 160ms ease;
}

:deep(.workflow-run-edge-pending .vue-flow__edge-path) {
  stroke: rgba(148, 163, 184, 0.42);
  opacity: 0.55;
}

:deep(.workflow-run-edge-passed .vue-flow__edge-path) {
  stroke: #22c55e;
  opacity: 0.92;
}

:deep(.workflow-run-edge-active .vue-flow__edge-path) {
  stroke: #60a5fa;
  opacity: 1;
  stroke-width: 3;
  filter: drop-shadow(0 0 6px rgba(96, 165, 250, 0.42));
}

:deep(.workflow-run-edge-failed .vue-flow__edge-path),
:deep(.workflow-run-edge-blocked .vue-flow__edge-path) {
  stroke: #ef4444;
  opacity: 0.92;
}

:deep(.vue-flow__controls) {
  z-index: 9;
  left: 206px;
  bottom: 16px;
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel) !important;
  box-shadow: var(--wf-shadow);
}

:deep(.vue-flow__controls button) {
  width: 28px;
  height: 28px;
  padding: 0;
  border-color: var(--wf-border) !important;
  background: var(--wf-panel) !important;
  color: var(--wf-text) !important;
}

:deep(.vue-flow__controls button:hover) {
  background: var(--wf-panel-soft) !important;
}

.workflow-editor.dark :deep(.wf-node) {
  background: #131d2b !important;
  border-color: #3f7ee8 !important;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.3) !important;
}

.workflow-editor.dark :deep(.wf-node.selected) {
  border-color: #6f9df5 !important;
  box-shadow: 0 0 0 4px rgba(96, 165, 250, 0.18), 0 14px 30px rgba(0, 0, 0, 0.34) !important;
}

.workflow-editor.dark :deep(.wf-node-header) {
  background: #1a2637 !important;
  border-bottom-color: rgba(148, 163, 184, 0.22) !important;
}

.workflow-editor.dark :deep(.wf-node-title) {
  color: #f8fafc !important;
}

.workflow-editor.dark :deep(.wf-node-content),
.workflow-editor.dark :deep(.wf-node-props-count) {
  color: #d6deea !important;
}

.workflow-editor.dark :deep(.wf-node-icon) {
  background: rgba(59, 130, 246, 0.16) !important;
  color: #8bb8ff !important;
}

.workflow-editor.dark :deep(.wf-node-badge) {
  background: rgba(148, 163, 184, 0.16) !important;
  color: #d8e2f0 !important;
}

@media (max-width: 1180px) {
  .workflow-topbar {
    grid-template-columns: 1fr;
    height: auto;
  }

  .workflow-editor {
    grid-template-rows: auto 34px minmax(0, 1fr) var(--workflow-bottom-height, 260px);
  }

  .workflow-main {
    grid-template-columns: 220px minmax(0, 1fr);
  }

  .workflow-properties {
    display: none;
  }
}
</style>
