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
        <button
          class="workflow-realtime-status"
          :class="`state-${workflowRuntimeConnectionState}`"
          type="button"
          :title="workflowRuntimeConnectionTitle"
          :aria-label="workflowRuntimeConnectionTitle"
          @click="workflowRuntimeBus.connected.value ? undefined : workflowRuntimeBus.connect()"
        >
          <span class="workflow-realtime-dot" />
          <span>{{ workflowRuntimeConnectionLabel }}</span>
          <Icon v-if="!workflowRuntimeBus.connected.value" name="i-heroicons-arrow-path" />
        </button>
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

    <UAlert
      v-if="workflowStructureIssues.length"
      color="warning"
      variant="soft"
      icon="i-heroicons-exclamation-triangle"
      :title="t('workflow.editor.structureIssueTitle')"
      :description="workflowStructureIssues.join(' ')"
      class="workflow-structure-alert"
    />

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
              :class="{ disabled: isPaletteItemDisabled(item.kind) }"
              :draggable="!isPaletteItemDisabled(item.kind)"
              :title="paletteItemTitle(item.kind)"
              :aria-disabled="isPaletteItemDisabled(item.kind)"
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
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.humanReviewBusinessTitle") }}</strong>
                  <span>{{ t("workflow.editor.humanReviewBusinessDescription") }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.fields.review_type')">
                  <USelect
                    v-model="selectedNode.data.props.review_type"
                    :items="reviewTypeOptions"
                    value-key="value"
                    label-key="label"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.approver_roles')">
                  <USelect
                    :model-value="primaryHumanReviewRole"
                    :items="humanReviewRoleOptions"
                    value-key="value"
                    label-key="label"
                    @update:model-value="updatePrimaryHumanReviewRole(String($event || ''))"
                  />
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
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.startNodeBusinessTitle") }}</strong>
                  <span>{{ t("workflow.editor.startNodeBusinessDescription") }}</span>
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
              </template>

              <template v-else-if="selectedNode.data.kind === 'skill.invoke'">
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.skillBusinessTitle") }}</strong>
                  <span>{{ t("workflow.editor.skillBusinessDescription") }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.editor.skillNodeSkillLabel')">
                  <div class="readonly-business-value">
                    {{ selectedNodeSkillLabel }}
                  </div>
                </UFormField>
                <div class="business-config-grid">
                  <div
                    v-for="entry in selectedNodeBusinessConfigEntries"
                    :key="entry.key"
                    class="business-config-item"
                  >
                    <span>{{ entry.label }}</span>
                    <strong>{{ entry.value }}</strong>
                  </div>
                </div>
              </template>

              <template v-else-if="selectedNode.data.kind === 'capability.invoke'">
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.capabilityBusinessTitle") }}</strong>
                  <span>{{ capabilityBusinessDescription }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.editor.nodeCapabilitySourceLabel')">
                  <USelectMenu
                    v-model="selectedNodeCapabilitySource"
                    :items="capabilitySourceSelectItems"
                    label-key="label"
                    :portal="false"
                    :content="runDialogSelectContent"
                    :ui="runDialogSelectUi"
                    class="w-full"
                    :placeholder="t('workflow.editor.capabilitySourceSelectPlaceholder')"
                    :search-input="{ placeholder: t('workflow.editor.capabilitySourceSearchPlaceholder') }"
                    :disabled="capabilitySourceSelectItems.length === 0"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.editor.nodeCapabilityModuleLabel')">
                  <USelectMenu
                    v-model="selectedNodeCapabilityModule"
                    :items="selectedNodeCapabilityModuleSelectItems"
                    label-key="label"
                    :portal="false"
                    :content="runDialogSelectContent"
                    :ui="runDialogSelectUi"
                    class="w-full"
                    :placeholder="t('workflow.editor.capabilityModuleSelectPlaceholder')"
                    :search-input="{ placeholder: t('workflow.editor.capabilityModuleSearchPlaceholder') }"
                    :disabled="!selectedNodeCapabilitySource || selectedNodeCapabilityModuleSelectItems.length === 0"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.editor.nodeCapabilityLabel')">
                  <USelectMenu
                    v-model="selectedNodeCapability"
                    :items="selectedNodeCapabilitySelectItems"
                    label-key="label"
                    :portal="false"
                    :content="runDialogSelectContent"
                    :ui="runDialogSelectUi"
                    class="w-full"
                    :placeholder="t('workflow.editor.capabilitySelectPlaceholder')"
                    :search-input="{ placeholder: t('workflow.editor.capabilitySearchPlaceholder') }"
                    :disabled="!selectedNodeCapabilitySource || !selectedNodeCapabilityModule || selectedNodeCapabilitySelectItems.length === 0"
                  />
                </UFormField>
                <UFormField class="properties-field" :label="t('workflow.fields.preferred_protocol')">
                  <USelect
                    v-model="selectedNode.data.props.preferred_protocol"
                    :items="selectedNodeProtocolOptions"
                    value-key="value"
                    label-key="label"
                    class="w-full"
                    :disabled="selectedNodeProtocolOptions.length === 0"
                  />
                </UFormField>
                <div v-if="capabilityOptionsError" class="properties-note state-error">
                  {{ capabilityOptionsError }}
                </div>
                <div v-else-if="selectedNodeCapabilityRecord" class="properties-note">
                  {{ t("workflow.editor.selectedCapabilityHint", { capability: capabilityOptionLabel(selectedNodeCapabilityRecord) }) }}
                </div>
              </template>

              <template v-else-if="selectedNode.data.kind === 'event.emit'">
                <div class="business-config-card">
                  <strong>{{ eventBusinessTitle }}</strong>
                  <span>{{ eventBusinessDescription }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.editor.eventTriggerLabel')">
                  <div class="readonly-business-value">
                    {{ eventTriggerLabel }}
                  </div>
                </UFormField>
              </template>

              <template v-else-if="selectedNode.data.kind === 'workflow.end'">
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.endNodeBusinessTitle") }}</strong>
                  <span>{{ t("workflow.editor.endNodeBusinessDescription") }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.editor.eventTriggerLabel')">
                  <div class="readonly-business-value">
                    {{ t("workflow.editor.eventTriggerEnd") }}
                  </div>
                </UFormField>
              </template>

              <template v-else-if="selectedNodeBusinessSummary">
                <div class="business-config-card">
                  <strong>{{ selectedNodeBusinessSummary.title }}</strong>
                  <span>{{ selectedNodeBusinessSummary.description }}</span>
                </div>
                <div
                  v-if="selectedNodeBusinessConfigEntries.length"
                  class="business-config-grid"
                >
                  <div
                    v-for="entry in selectedNodeBusinessConfigEntries"
                    :key="entry.key"
                    class="business-config-item"
                  >
                    <span>{{ entry.label }}</span>
                    <strong>{{ entry.value }}</strong>
                  </div>
                </div>
                <UFormField
                  v-if="selectedNodeBusinessSummary.result"
                  class="properties-field"
                  :label="t('workflow.editor.nodeBusinessResultLabel')"
                >
                  <div class="readonly-business-value">
                    {{ selectedNodeBusinessSummary.result }}
                  </div>
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

              <div v-if="advancedNodeConfigEntries.length" class="advanced-config-section">
                <button
                  class="advanced-config-toggle"
                  type="button"
                  @click="showAdvancedNodeConfig = !showAdvancedNodeConfig"
                >
                  <span>{{ t("workflow.editor.advancedConfig") }}</span>
                  <Icon :name="showAdvancedNodeConfig ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" />
                </button>
                <div v-if="showAdvancedNodeConfig" class="advanced-config-list">
                  <div
                    v-for="entry in advancedNodeConfigEntries"
                    :key="entry.key"
                    class="advanced-config-row"
                  >
                    <span>{{ entry.label }}</span>
                    <code>{{ entry.value }}</code>
                  </div>
                  <p>{{ t("workflow.editor.advancedConfigHint") }}</p>
                </div>
              </div>
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
                <div class="runtime-summary-card">
                  <span>{{ t("workflow.editor.reviewPayload") }}</span>
                  <div class="runtime-summary-list">
                    <div
                      v-for="entry in selectedReviewPayloadEntries"
                      :key="entry.key"
                      class="runtime-summary-row"
                    >
                      <span>{{ entry.label }}</span>
                      <strong>{{ entry.value }}</strong>
                    </div>
                  </div>
                </div>
              </template>
              <template v-else-if="selectedNodeRunStep">
                <div class="runtime-summary-card">
                  <span>{{ t("workflow.editor.nodeRunRecord") }}</span>
                  <div class="runtime-summary-list">
                    <div
                      v-for="entry in selectedNodeRunSummaryEntries"
                      :key="entry.key"
                      class="runtime-summary-row"
                    >
                      <span>{{ entry.label }}</span>
                      <strong>{{ entry.value }}</strong>
                    </div>
                  </div>
                </div>
              </template>
              <div v-else-if="selectedNode.data.kind === 'human.review' && latestRunState === 'waiting'" class="runtime-empty-card">
                {{ t("workflow.editor.noReviewTaskForNode") }}
              </div>
              <div v-else class="runtime-empty-card">
                {{ t("workflow.editor.noRuntimeForNode") }}
              </div>
              <div v-if="selectedRuntimeDiagnosticsEntries.length" class="advanced-config-section">
                <button
                  class="advanced-config-toggle"
                  type="button"
                  @click="showRuntimeDiagnostics = !showRuntimeDiagnostics"
                >
                  <span>{{ t("workflow.editor.runtimeDiagnostics") }}</span>
                  <Icon :name="showRuntimeDiagnostics ? 'i-heroicons-chevron-up' : 'i-heroicons-chevron-down'" />
                </button>
                <div v-if="showRuntimeDiagnostics" class="advanced-config-list">
                  <div
                    v-for="entry in selectedRuntimeDiagnosticsEntries"
                    :key="entry.key"
                    class="advanced-config-row"
                  >
                    <span>{{ entry.label }}</span>
                    <code>{{ entry.value }}</code>
                  </div>
                  <p>{{ t("workflow.editor.runtimeDiagnosticsHint") }}</p>
                </div>
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
            <UFormField :label="t('workflow.editor.runCapabilitySourceLabel')">
              <USelectMenu
                v-model="selectedRunCapabilitySource"
                :items="capabilitySourceSelectItems"
                label-key="label"
                :portal="runDialogSelectPortal"
                :content="runDialogSelectContent"
                :ui="runDialogSelectUi"
                class="w-full"
                :loading="capabilityOptionsLoading"
                :disabled="capabilityOptionsLoading || capabilitySourceSelectItems.length === 0"
                :placeholder="t('workflow.editor.capabilitySourceSelectPlaceholder')"
                :search-input="{ placeholder: t('workflow.editor.capabilitySourceSearchPlaceholder') }"
              />
            </UFormField>
            <UFormField :label="t('workflow.editor.runCapabilityModuleLabel')">
              <USelectMenu
                v-model="selectedRunCapabilityModule"
                :items="runCapabilityModuleSelectItems"
                label-key="label"
                :portal="runDialogSelectPortal"
                :content="runDialogSelectContent"
                :ui="runDialogSelectUi"
                class="w-full"
                :loading="capabilityOptionsLoading"
                :disabled="capabilityOptionsLoading || !selectedRunCapabilitySource || runCapabilityModuleSelectItems.length === 0"
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
                :disabled="capabilityOptionsLoading || !selectedRunCapabilitySource || !selectedRunCapabilityModule || capabilitySelectItems.length === 0"
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
  screenToFlowCoordinate,
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
const runtimeEventQueue: WorkflowRuntimeEvent[] = [];
let runtimeEventQueueProcessing = false;
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
const selectedRunCapabilitySource = ref<SelectOption | null>(null);
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
const showAdvancedNodeConfig = ref(false);
const showRuntimeDiagnostics = ref(false);
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
  { label: t("workflow.protocol.agentTool"), value: "agent_tool" },
]);

const humanReviewRoleOptions = computed(() => {
  const values = new Set(["workflow_reviewer"]);
  for (const role of currentHumanReviewRoles.value) {
    values.add(role);
  }
  return [...values].map((value) => ({
    label: roleDisplayName(value),
    value,
  }));
});

const workflowRunnableCapabilities = computed(() =>
  [...capabilityOptions.value].filter((capability) => isWorkflowRunnableBusinessCapability(capability))
);

const platformCapabilitySourceKey = "platform";

type CapabilityModuleOptionMeta = {
  label: string;
  count: number;
  sourceKey: string;
  moduleKey: string;
};

const capabilitySourceSelectItems = computed<SelectOption[]>(() => {
  const sources = new Map<string, { label: string; count: number; plugin: boolean }>();
  for (const capability of workflowRunnableCapabilities.value) {
    const sourceKey = capabilitySourceKey(capability);
    const current = sources.get(sourceKey);
    if (current) {
      current.count += 1;
      continue;
    }
    sources.set(sourceKey, {
      label: capabilitySourceLabel(capability),
      count: 1,
      plugin: capability.source === "plugin",
    });
  }
  return [...sources.entries()]
    .sort((left, right) => {
      if (left[0] === platformCapabilitySourceKey) return -1;
      if (right[0] === platformCapabilitySourceKey) return 1;
      return left[1].label.localeCompare(right[1].label);
    })
    .map(([value, item]) => ({
      label: item.plugin
        ? t("workflow.editor.pluginCapabilitySourceOptionLabel", { plugin: item.label, count: item.count })
        : t("workflow.editor.platformCapabilitySourceOptionLabel", { platform: item.label, count: item.count }),
      value,
    }));
});

const runCapabilityModuleSelectItems = computed<SelectOption[]>(() =>
  buildCapabilityModuleSelectItems(selectedRunCapabilitySource.value?.value || "")
);

const capabilitySelectItems = computed<SelectOption[]>(() =>
  workflowRunnableCapabilities.value
    .filter((capability) => capabilityMatchesModuleOption(capability, selectedRunCapabilityModule.value?.value || ""))
    .sort((left, right) => Number(hasLocalizedCapabilityName(right)) - Number(hasLocalizedCapabilityName(left)))
    .map((capability) => ({
      label: capabilityOptionLabel(capability),
      value: capability.capabilityId,
    }))
);

const selectedNodeCapabilityRecord = computed(() => {
  const capabilityID = String(selectedNode.value?.data?.props?.capability_id || "").trim();
  if (!capabilityID || isRuntimeTemplateValue(capabilityID)) return null;
  return workflowRunnableCapabilities.value.find((capability) => capability.capabilityId === capabilityID) || null;
});

const selectedNodeCapabilitySource = computed<SelectOption | null>({
  get() {
    const sourceKey =
      (selectedNodeCapabilityRecord.value ? capabilitySourceKey(selectedNodeCapabilityRecord.value) : "") ||
      String(selectedNode.value?.data?.props?.capability_source || "").trim();
    if (!sourceKey) return null;
    return capabilitySourceSelectItems.value.find((item) => item.value === sourceKey) || null;
  },
  set(sourceOption) {
    const sourceKey = sourceOption?.value || "";
    const node = selectedNode.value;
    if (!node || node.data.kind !== "capability.invoke") return;
    if (!sourceKey) {
      clearSelectedNodeCapability();
      return;
    }
    node.data.props = {
      ...(node.data.props || {}),
      capability_source: sourceKey,
      capability_source_label: sourceOption?.label || "",
      capability_module: "",
      capability_module_label: "",
      capability_id: "",
      capability_label: "",
      preferred_protocol: "",
    };
  },
});

const selectedNodeCapabilityModuleSelectItems = computed<SelectOption[]>(() =>
  buildCapabilityModuleSelectItems(selectedNodeCapabilitySource.value?.value || "")
);

const selectedNodeCapabilityModule = computed<SelectOption | null>({
  get() {
    const moduleKey =
      (selectedNodeCapabilityRecord.value ? capabilityModuleOptionValue(selectedNodeCapabilityRecord.value) : "") ||
      String(selectedNode.value?.data?.props?.capability_module || "").trim();
    if (!moduleKey) return null;
    return selectedNodeCapabilityModuleSelectItems.value.find((item) => item.value === moduleKey) || null;
  },
  set(moduleOption) {
    const moduleKey = moduleOption?.value || "";
    const node = selectedNode.value;
    if (!node || node.data.kind !== "capability.invoke") return;
    if (!moduleKey) {
      clearSelectedNodeCapability();
      return;
    }
    node.data.props = {
      ...(node.data.props || {}),
      capability_module: moduleKey,
      capability_module_label: moduleOption?.label || "",
      capability_id: "",
      capability_label: "",
      preferred_protocol: "",
    };
  },
});

const selectedNodeCapabilitySelectItems = computed<SelectOption[]>(() => {
  const moduleKey = selectedNodeCapabilityModule.value?.value || "";
  if (!moduleKey) return [];
  return workflowRunnableCapabilities.value
    .filter((capability) => capabilityMatchesModuleOption(capability, moduleKey))
    .sort((left, right) => Number(hasLocalizedCapabilityName(right)) - Number(hasLocalizedCapabilityName(left)))
    .map((capability) => ({
      label: capabilityOptionLabel(capability),
      value: capability.capabilityId,
    }));
});

const selectedNodeCapability = computed<SelectOption | null>({
  get() {
    const capabilityID = selectedNodeCapabilityRecord.value?.capabilityId || "";
    if (!capabilityID) return null;
    return selectedNodeCapabilitySelectItems.value.find((item) => item.value === capabilityID) || null;
  },
  set(capabilityOption) {
    applyCapabilityToSelectedNode(capabilityOption?.value || "");
  },
});

const selectedNodeProtocolOptions = computed<SelectOption[]>(() => {
  if (!selectedNodeCapabilityRecord.value) return [];
  const channels = selectedNodeCapabilityRecord.value?.protocols
    ?.map((protocol) => String(protocol.channel || "").trim())
    .filter(isWorkflowCapabilityInvokeProtocol) || [];
  const values = [...new Set(channels)];
  return values.map((value) => ({
    label: protocolLabel(value),
    value,
  }));
});

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
  content: "z-[90] max-h-72 overflow-y-auto",
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
  if (selectedCapability && capabilityMatchesModuleOption(selectedCapability, moduleKey)) return;
  selectedRunCapability.value = capabilitySelectItems.value[0] || null;
  approvalDebugForm.capability_id = selectedRunCapability.value?.value || "";
});

watch(selectedRunCapabilitySource, (sourceOption) => {
  if (!isApprovalGuardedCapabilityWorkflow.value) return;
  const sourceKey = sourceOption?.value || "";
  const selectedCapabilityID = selectedRunCapability.value?.value || "";
  const selectedCapability = capabilityOptions.value.find((item) => item.capabilityId === selectedCapabilityID);
  if (selectedCapability && capabilitySourceKey(selectedCapability) === sourceKey) return;
  selectedRunCapabilityModule.value = runCapabilityModuleSelectItems.value[0] || null;
  selectedRunCapability.value = capabilitySelectItems.value[0] || null;
  approvalDebugForm.capability_id = selectedRunCapability.value?.value || "";
});

const currentHumanReviewRoles = computed(() => {
  const roles = selectedNode.value?.data?.props?.approver_policy?.roles;
  return Array.isArray(roles) ? roles.map((role) => String(role).trim()).filter(Boolean) : [];
});
const primaryHumanReviewRole = computed(() => currentHumanReviewRoles.value[0] || "workflow_reviewer");

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
const workflowRuntimeConnectionTitle = computed(() => {
  const reason = String(workflowRuntimeBus.lastError.value || "").trim();
  if (!reason) return workflowRuntimeConnectionLabel.value;
  return t("workflow.editor.runtimeDisconnectedWithReason", {
    reason: workflowRuntimeErrorLabel(reason),
  });
});
const workflowRuntimeConnectionState = computed(() => {
  if (workflowRuntimeBus.connected.value) return "connected";
  if (workflowRuntimeBus.connecting.value) return "connecting";
  return "disconnected";
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
  if (latestRun.value) {
    const stepByID = new Map(steps.map((step) => [step.step_id, step]));
    const rows = nodes.value.map((node) => {
      const step = stepByID.get(node.id);
      if (step) {
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
      }
      const state = effectiveNodeRunState(node.id);
      return {
        id: node.id,
        label: displayLabel(node.data.label),
        kindLabel: node.data.kind ? t("workflow.editor.stepKindWithValue", { kind: getKindLabel(node.data.kind) }) : t("workflow.editor.unknownNodeKind"),
        stateLabel: t(`workflow.state.${state}`),
        stateClass: `state-${state}`,
        badgeColor: runStateColor(state),
        message: state === "skipped" ? t("workflow.editor.stepSkippedByBranch") : "",
      };
    });
    for (const step of steps) {
      if (nodes.value.some((node) => node.id === step.step_id)) continue;
      const state = normalizeEffectiveStepState(step.state);
      const startedAt = formatRunTimestamp(step.started_at || step.scheduled_at || "");
      const completedAt = formatRunTimestamp(step.completed_at || "");
      const error = step.error_message || step.failure_reason || step.error_code || "";
      rows.push({
        id: step.step_id,
        label: stepDisplayName(step.step_id),
        kindLabel: stepKindLabel(step),
        stateLabel: t(`workflow.state.${state}`),
        stateClass: `state-${state}`,
        badgeColor: runStateColor(state),
        message: stepRunMessage(state, startedAt, completedAt, error),
      });
    }
    return rows;
  }
  return nodes.value.map((node) => ({
    id: node.id,
    label: displayLabel(node.data.label),
    kindLabel: node.data.kind ? t("workflow.editor.stepKindWithValue", { kind: getKindLabel(node.data.kind) }) : t("workflow.editor.unknownNodeKind"),
    stateLabel: t("workflow.editor.noRuns"),
    stateClass: "",
    badgeColor: runStateColor(""),
    message: "",
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

const startNodeCount = computed(() => nodes.value.filter((node) => node.data?.kind === "input.capture").length);
const endNodeCount = computed(() => nodes.value.filter((node) => node.data?.kind === "workflow.end").length);
const workflowStructureIssues = computed(() => {
  const issues: string[] = [];
  if (startNodeCount.value !== 1) {
    issues.push(t("workflow.editor.structureStartIssue", { count: startNodeCount.value }));
  }
  if (endNodeCount.value !== 1) {
    issues.push(t("workflow.editor.structureEndIssue", { count: endNodeCount.value }));
  }
  return issues;
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

const selectedNodeRunState = computed(() => effectiveNodeRunState(selectedNode.value?.id || ""));
const selectedNodeRunStateLabel = computed(() => {
  if (!latestRun.value) return t("workflow.editor.noRuns");
  return t(`workflow.state.${selectedNodeRunState.value || "pending"}`);
});

const capabilityBusinessDescription = computed(() => {
  const capabilityID = String(selectedNode.value?.data?.props?.capability_id || "");
  if (isRuntimeTemplateValue(capabilityID)) return t("workflow.editor.capabilityBusinessRuntimeDescription");
  return t("workflow.editor.capabilityBusinessFixedDescription");
});

const eventBusinessTitle = computed(() => {
  if (isRejectedEventNode(selectedNode.value?.id || "", selectedNode.value?.data?.props?.topic)) {
    return t("workflow.editor.eventRejectedBusinessTitle");
  }
  return t("workflow.editor.eventCompletedBusinessTitle");
});

const eventBusinessDescription = computed(() => {
  if (isRejectedEventNode(selectedNode.value?.id || "", selectedNode.value?.data?.props?.topic)) {
    return t("workflow.editor.eventRejectedBusinessDescription");
  }
  return t("workflow.editor.eventCompletedBusinessDescription");
});

const eventTriggerLabel = computed(() => {
  if (isRejectedEventNode(selectedNode.value?.id || "", selectedNode.value?.data?.props?.topic)) {
    return t("workflow.editor.eventTriggerRejected");
  }
  return t("workflow.editor.eventTriggerCompleted");
});

const selectedNodeSkillLabel = computed(() => {
  const skillID = String(selectedNode.value?.data?.props?.skill_id || "").trim();
  if (!skillID) return t("workflow.editor.noSkillConfigured");
  return humanizeSkillID(skillID);
});

const selectedNodeBusinessSummary = computed(() => {
  const kind = selectedNode.value?.data?.kind || "";
  const summaryKeyByKind: Record<string, string> = {
    "metadata.classify": "metadataClassify",
    "knowledge.stage": "knowledgeStage",
    "knowledge.publish": "knowledgePublish",
    "decision.gateway": "decisionGateway",
    "parallel.fanout": "parallelFanout",
    "parallel.join": "parallelJoin",
    "compensation.rollback": "compensationRollback",
  };
  const summaryKey = summaryKeyByKind[kind];
  if (!summaryKey) return null;
  return {
    title: t(`workflow.editor.nodeBusiness.${summaryKey}.title`),
    description: t(`workflow.editor.nodeBusiness.${summaryKey}.description`),
    result: t(`workflow.editor.nodeBusiness.${summaryKey}.result`),
  };
});

const selectedNodeBusinessConfigEntries = computed(() => {
  const node = selectedNode.value;
  if (!node) return [];
  const props = node.data.props || {};
  const kind = node.data.kind || "";
  const entry = (key: string, labelKey: string, value: unknown) => ({
    key,
    label: t(labelKey),
    value: formatBusinessConfigValue(key, value),
  });
  switch (kind) {
    case "skill.invoke":
      return [
        entry("input_path", "workflow.editor.businessInputSource", props.input_path),
        entry("output_path", "workflow.editor.businessOutputTarget", props.output_path),
      ];
    case "metadata.classify":
      return [
        entry("taxonomy_namespace", "workflow.fields.taxonomy_namespace", props.taxonomy_namespace),
        entry("tag_namespace", "workflow.fields.tag_namespace", props.tag_namespace),
        entry("dictionary_namespace", "workflow.fields.dictionary_namespace", props.dictionary_namespace),
        entry("resource_type_namespace", "workflow.fields.resource_type_namespace", props.resource_type_namespace),
        entry("output_path", "workflow.editor.businessOutputTarget", props.output_path),
      ];
    case "knowledge.stage":
      return [
        entry("knowledge_space_uuid", "workflow.fields.knowledge_space_uuid", props.knowledge_space_uuid),
        entry("draft_schema_ref", "workflow.fields.draft_schema_ref", props.draft_schema_ref),
        entry("input_path", "workflow.editor.businessInputSource", props.input_path),
      ];
    case "knowledge.publish":
      return [
        entry("knowledge_space_uuid", "workflow.fields.knowledge_space_uuid", props.knowledge_space_uuid),
        entry("publish_policy", "workflow.fields.publish_policy", props.publish_policy),
        entry("draft_refs_path", "workflow.fields.draft_refs_path", props.draft_refs_path),
      ];
    case "decision.gateway":
      return [
        entry("default_route", "workflow.fields.default_route", props.default_route),
        entry("condition_source_path", "workflow.fields.condition_source_path", props.condition_source_path),
      ];
    default:
      return [];
  }
});

const advancedNodeConfigEntries = computed(() => {
  const props = selectedNode.value?.data?.props || {};
  const hiddenKeys = hiddenBusinessConfigKeys(selectedNode.value?.data?.kind || "");
  return Object.entries(props)
    .filter(([key]) => !hiddenKeys.has(key))
    .map(([key, value]) => ({
      key,
      label: schemaFieldLabel(key),
      value: formatTechnicalValue(value),
    }));
});

const selectedReviewPayloadEntries = computed(() => {
  const payload = selectedNodeReviewTask.value?.payload || {};
  return [
    {
      key: "capability",
      label: t("workflow.editor.runtimeCapability"),
      value: capabilityLabelForID(String(payload.capability_id || "")),
    },
    {
      key: "reason",
      label: t("workflow.fields.reason"),
      value: executionReasonLabel(String(payload.request?.reason || "")),
    },
    {
      key: "dry_run",
      label: t("workflow.fields.dry_run"),
      value: payload.request?.payload?.dry_run ? t("common.yes") : t("common.no"),
    },
    {
      key: "note",
      label: t("workflow.fields.note"),
      value: String(payload.request?.payload?.note || t("workflow.editor.none")),
    },
  ];
});

const selectedNodeRunSummaryEntries = computed(() => {
  const step = selectedNodeRunStep.value;
  if (!step) return [];
  return [
    {
      key: "state",
      label: t("workflow.editor.currentNodeState"),
      value: t(`workflow.state.${normalizeEffectiveStepState(step.state)}`),
    },
    {
      key: "started",
      label: t("workflow.editor.startedAt"),
      value: formatRunTimestamp(step.started_at || step.scheduled_at || "") || t("workflow.editor.notStarted"),
    },
    {
      key: "completed",
      label: t("workflow.editor.completedAt"),
      value: formatRunTimestamp(step.completed_at || "") || t("workflow.editor.notCompleted"),
    },
    {
      key: "attempt",
      label: t("workflow.editor.attempt"),
      value: String(step.attempt || 1),
    },
  ];
});

const selectedRuntimeDiagnosticsEntries = computed(() => {
  const entries: Array<{ key: string; label: string; value: string }> = [];
  if (selectedNodeReviewTask.value?.payload) {
    entries.push({
      key: "review_payload",
      label: t("workflow.editor.reviewPayload"),
      value: formatTechnicalValue(selectedNodeReviewTask.value.payload),
    });
  }
  if (selectedNodeRunStep.value) {
    entries.push({
      key: "step_record",
      label: t("workflow.editor.nodeRunRecord"),
      value: formatStepRunPayload(selectedNodeRunStep.value),
    });
  }
  return entries;
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

function executionReasonLabel(reason: string) {
  if (!reason.trim()) return t("workflow.editor.none");
  const key = `workflow.executionReason.${camelCase(reason)}`;
  return te(key) ? t(key) : humanizeModuleKey(reason);
}

function workflowRuntimeErrorLabel(reason: string) {
  const key = `workflow.runtimeError.${camelCase(reason)}`;
  return te(key) ? t(key) : reason;
}

function paletteCategoryForKind(kind: string) {
  if (kind === "input.capture" || kind === "workflow.end" || kind === "event.emit") return "input_output";
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

function updatePrimaryHumanReviewRole(value: string) {
  if (!selectedNode.value) return;
  selectedNode.value.data.props.approver_policy = {
    ...(selectedNode.value.data.props.approver_policy || {}),
    roles: value ? [value] : [],
  };
}

function isRuntimeTemplateValue(value: unknown) {
  return typeof value === "string" && /^\$\{[a-zA-Z0-9_]+\}$/.test(value.trim());
}

function roleDisplayName(role: string) {
  const key = `workflow.role.${camelCase(role)}`;
  if (te(key)) return t(key);
  return humanizeModuleKey(role);
}

function capabilityLabelForID(capabilityID: string) {
  if (!capabilityID.trim()) return t("workflow.editor.noCapabilityConfigured");
  const capability = capabilityOptions.value.find((item) => item.capabilityId === capabilityID);
  if (capability) return capabilityOptionLabel(capability);
  const localizedKey = capabilityI18nKey(capabilityID);
  if (te(localizedKey)) return t(localizedKey);
  return t("workflow.editor.fixedCapabilityConfigured");
}

function isRejectedEventNode(nodeID: string, topic: unknown) {
  const topicValue = String(topic || "").toLowerCase();
  return nodeID.toLowerCase().includes("reject") || topicValue.includes("reject");
}

function isPaletteItemDisabled(kind: string) {
  return (kind === "input.capture" && startNodeCount.value >= 1) || (kind === "workflow.end" && endNodeCount.value >= 1);
}

function paletteItemTitle(kind: string) {
  if (kind === "input.capture" && startNodeCount.value >= 1) return t("workflow.editor.startNodeAlreadyExists");
  if (kind === "workflow.end" && endNodeCount.value >= 1) return t("workflow.editor.endNodeAlreadyExists");
  return "";
}

function humanizeSkillID(skillID: string) {
  const localizedKey = `workflow.skill.${camelCase(skillID.replace(/[^a-zA-Z0-9]+/g, "_"))}`;
  if (te(localizedKey)) return t(localizedKey);
  return humanizeModuleKey(skillID);
}

function hiddenBusinessConfigKeys(kind: string) {
  const shared = new Set(["capability_label", "capability_module_label"]);
  const byKind: Record<string, string[]> = {
    "skill.invoke": ["skill_id", "input_path", "output_path"],
    "metadata.classify": ["taxonomy_namespace", "tag_namespace", "dictionary_namespace", "resource_type_namespace", "input_path", "output_path"],
    "knowledge.stage": ["knowledge_space_uuid", "draft_schema_ref", "input_path", "output_path"],
    "knowledge.publish": ["knowledge_space_uuid", "draft_refs_path", "review_result_path", "publish_policy"],
    "decision.gateway": ["routes", "default_route", "condition_source_path"],
  };
  for (const key of byKind[kind] || []) {
    shared.add(key);
  }
  return shared;
}

function formatBusinessConfigValue(key: string, value: unknown) {
  if (value === undefined || value === null || value === "") return t("workflow.editor.notConfigured");
  if (key.endsWith("_path") || key === "draft_refs_path" || key === "condition_source_path") {
    return businessPathLabel(String(value));
  }
  if (key === "skill_id") return humanizeSkillID(String(value));
  if (key === "knowledge_space_uuid" && isRuntimeTemplateValue(value)) return t("workflow.editor.runInputProvided");
  if (key.endsWith("_namespace")) return humanizeModuleKey(String(value));
  if (key.endsWith("_route") || key === "default_route") return stepDisplayName(String(value));
  if (key === "publish_policy") {
    const i18nKey = `workflow.publishPolicy.${camelCase(String(value))}`;
    return te(i18nKey) ? t(i18nKey) : humanizeModuleKey(String(value));
  }
  if (typeof value === "object") return t("workflow.editor.configured");
  return humanizeModuleKey(String(value));
}

function businessPathLabel(path: string) {
  const key = `workflow.path.${camelCase(path.replace(/^\$\./, ""))}`;
  if (te(key)) return t(key);
  if (path.startsWith("$.")) return t("workflow.editor.workflowVariablePath");
  return path;
}

function formatTechnicalValue(value: unknown) {
  if (value === undefined || value === null) return "";
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  return String(value);
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
  const paletteItem = palette.value.find((item) => item.id === paletteId);
  if (paletteItem && isPaletteItemDisabled(paletteItem.kind)) {
    event.preventDefault();
    return;
  }
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
  const paletteItem = palette.value.find((item) => item.id === paletteId);
  if (paletteItem && isPaletteItemDisabled(paletteItem.kind)) {
    toast.add({
      title: paletteItemTitle(paletteItem.kind),
      color: "warning",
    });
    return;
  }

  const cursorPosition = screenToFlowCoordinate({
    x: event.clientX,
    y: event.clientY,
  });
  const nodeSize = kinds.value[paletteId]?.ui?.size || { w: 240, h: 96 };
  const position = {
    x: cursorPosition.x - (nodeSize.w || 240) / 2,
    y: cursorPosition.y - (nodeSize.h || 96) / 2,
  };

  const newNode = addNodeFromPalette(paletteId, position);
  if (newNode) {
    nodes.value = nodes.value.map((node) => ({ ...node, selected: false }));
    const selectedNewNode = { ...newNode, selected: true };
    addNodes([selectedNewNode]);
    normalizeSelectedNodeProps(selectedNewNode);
    selectedNode.value = selectedNewNode;
    propertiesTab.value = "config";
    if (selectedNewNode.data.kind === "capability.invoke") {
      void loadCapabilityReferenceData({ notify: false }).then(normalizeSelectedNodePreferredProtocol);
    }
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
  const minHeight = 160;
  const maxHeight = 520;
  document.body.style.cursor = "ns-resize";
  document.body.style.userSelect = "none";

  const onPointerMove = (moveEvent: PointerEvent) => {
    const nextHeight = window.innerHeight - moveEvent.clientY;
    bottomPanelHeight.value = Math.min(maxHeight, Math.max(minHeight, nextHeight));
  };

  const onPointerUp = () => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    stopBottomResize?.();
    stopBottomResize = null;
  };

  window.addEventListener("pointermove", onPointerMove);
  window.addEventListener("pointerup", onPointerUp, { once: true });
  window.addEventListener("pointercancel", onPointerUp, { once: true });
  stopBottomResize = () => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerUp);
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
  fitView({ padding: 0.28, duration: 0 });
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
  if (node.data.kind === "capability.invoke") {
    void loadCapabilityReferenceData({ notify: false }).then(normalizeSelectedNodePreferredProtocol);
  }
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
  if (!isApprovalGuardedCapabilityWorkflow.value) return;
  await loadCapabilityReferenceData({ notify: true });
}

async function loadCapabilityReferenceData(options: { notify?: boolean } = {}) {
  if (capabilityOptions.value.length || capabilityOptionsLoading.value) return;
  const notify = options.notify ?? true;
  capabilityOptionsLoading.value = true;
  capabilityOptionsError.value = "";
  try {
    const result = await PlatformCapabilityService.listModules({ source: "all", pageSize: 200 });
    capabilityOptions.value = result.modules.flatMap((module) =>
      module.capabilities.map((capability) => ({
        ...capability,
        moduleDisplayName: module.displayName || module.module,
      }))
    );
    syncRunDialogSelects();
    normalizeSelectedNodePreferredProtocol();
    if (!capabilitySourceSelectItems.value.length) {
      capabilityOptionsError.value = t("workflow.editor.capabilityOptionsEmpty");
      if (notify) {
        toast.add({
          title: t("workflow.editor.capabilityOptionsLoadFailed"),
          description: capabilityOptionsError.value,
          color: "error",
        });
      }
    }
  } catch (err: any) {
    capabilityOptionsError.value = err?.message || t("workflow.editor.capabilityOptionsLoadFailed");
    if (notify) {
      toast.add({
        title: t("workflow.editor.capabilityOptionsLoadFailed"),
        description: capabilityOptionsError.value,
        color: "error",
      });
    }
  } finally {
    capabilityOptionsLoading.value = false;
  }
}

function capabilityOptionLabel(capability: RunCapabilityOption) {
  const localizedKey = capabilityI18nKey(capability.capabilityId);
  return te(localizedKey) ? t(localizedKey) : readableCapabilityTitle(capability);
}

function protocolLabel(value: string) {
  const key = `workflow.protocol.${camelCase(value)}`;
  return te(key) ? t(key) : humanizeModuleKey(value);
}

function isWorkflowCapabilityInvokeProtocol(value: string) {
  return ["rest", "grpc", "agent_tool"].includes(String(value || "").trim().toLowerCase());
}

function hasWorkflowCapabilityInvokeProtocol(capability: RunCapabilityOption) {
  return (capability.protocols || []).some((protocol) =>
    isWorkflowCapabilityInvokeProtocol(String(protocol.channel || ""))
  );
}

function resolveDefaultWorkflowCapabilityProtocol(capability: RunCapabilityOption) {
  const channels = (capability.protocols || [])
    .map((protocol) => String(protocol.channel || "").trim().toLowerCase())
    .filter(isWorkflowCapabilityInvokeProtocol);
  const preferred = String(capability.preferredProtocol || "").trim().toLowerCase();
  if (isWorkflowCapabilityInvokeProtocol(preferred) && channels.includes(preferred)) return preferred;
  for (const protocol of ["rest", "grpc", "agent_tool"]) {
    if (channels.includes(protocol)) return protocol;
  }
  return "";
}

function normalizeSelectedNodePreferredProtocol() {
  const node = selectedNode.value;
  const capability = selectedNodeCapabilityRecord.value;
  if (!node || node.data.kind !== "capability.invoke" || !capability) return;
  const current = String(node.data.props?.preferred_protocol || "").trim().toLowerCase();
  const available = selectedNodeProtocolOptions.value.map((item) => item.value);
  if (available.includes(current)) return;
  const next = resolveDefaultWorkflowCapabilityProtocol(capability);
  if (!next) return;
  node.data.props = {
    ...(node.data.props || {}),
    preferred_protocol: next,
  };
}

function applyCapabilityToSelectedNode(capabilityID: string) {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "capability.invoke") return;
  const capability = workflowRunnableCapabilities.value.find((item) => item.capabilityId === capabilityID);
  if (!capability) {
    clearSelectedNodeCapability();
    return;
  }
  const preferredProtocol = resolveDefaultWorkflowCapabilityProtocol(capability);
  node.data.props = {
    ...(node.data.props || {}),
    capability_source: capabilitySourceKey(capability),
    capability_source_label: capabilitySourceLabel(capability),
    capability_module: capabilityModuleOptionValue(capability),
    capability_module_label: capabilityModuleLabel(capability),
    capability_id: capability.capabilityId,
    capability_label: capabilityOptionLabel(capability),
    preferred_protocol: preferredProtocol,
    input_path: String(node.data.props?.input_path || "request.payload"),
    output_path: String(node.data.props?.output_path || "capability_result"),
  };
}

function clearSelectedNodeCapability() {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "capability.invoke") return;
  node.data.props = {
    ...(node.data.props || {}),
    capability_source: "",
    capability_source_label: "",
    capability_module: "",
    capability_module_label: "",
    capability_id: "",
    capability_label: "",
    preferred_protocol: "",
  };
}

function capabilityModuleLabel(capability: RunCapabilityOption) {
  const moduleKey = `workflow.capabilityModule.${camelCase(capabilityBusinessModuleKey(capability))}`;
  if (te(moduleKey)) return t(moduleKey);
  if (capability.source === "plugin") return humanizeModuleKey(capabilityBusinessModuleKey(capability));
  const displayName = capability.moduleDisplayName?.trim() || "";
  if (displayName && displayName !== capability.module) return displayName;
  return humanizeModuleKey(capability.module || "");
}

function capabilitySourceKey(capability: RunCapabilityOption) {
  if (capability.source === "plugin") {
    return String(capability.pluginId || "plugin").trim() || "plugin";
  }
  return platformCapabilitySourceKey;
}

function capabilitySourceLabel(capability: RunCapabilityOption) {
  if (capability.source === "plugin") return humanizePluginID(capability.pluginId || capability.capabilityId);
  return t("workflow.editor.platformCapabilitySourceName");
}

function capabilityBusinessModuleKey(capability: RunCapabilityOption) {
  if (capability.source !== "plugin") return capability.module || "corex";
  const capabilityID = String(capability.capabilityId || "").trim();
  const pluginID = String(capability.pluginId || "").trim();
  let businessPart = capabilityID;
  if (pluginID && capabilityID.startsWith(`${pluginID}.`)) {
    businessPart = capabilityID.slice(pluginID.length + 1);
  } else {
    businessPart = businessPart
      .replace(/^com\.powerx\.plugins\./, "")
      .replace(/^com\./, "");
  }
  const segment = businessPart.split(".").map((item) => item.trim()).find(Boolean);
  return normalizeCapabilitySegment(segment || capability.module || "plugin");
}

function capabilityModuleOptionValue(capability: RunCapabilityOption) {
  return `${capabilitySourceKey(capability)}::${capabilityBusinessModuleKey(capability)}`;
}

function capabilityMatchesModuleOption(capability: RunCapabilityOption, optionValue: string) {
  return capabilityModuleOptionValue(capability) === optionValue;
}

function buildCapabilityModuleSelectItems(sourceKey: string) {
  if (!sourceKey) return [];
  const modules = new Map<string, CapabilityModuleOptionMeta>();
  for (const capability of workflowRunnableCapabilities.value) {
    if (capabilitySourceKey(capability) !== sourceKey) continue;
    const moduleKey = capabilityBusinessModuleKey(capability);
    const optionValue = capabilityModuleOptionValue(capability);
    const current = modules.get(optionValue);
    if (current) {
      current.count += 1;
      continue;
    }
    modules.set(optionValue, {
      label: capabilityModuleLabel(capability),
      count: 1,
      sourceKey,
      moduleKey,
    });
  }
  return [...modules.entries()]
    .sort((left, right) => left[1].label.localeCompare(right[1].label))
    .map(([value, item]) => ({
      label: t("workflow.editor.capabilityModuleOptionLabel", { module: item.label, count: item.count }),
      value,
    }));
}

function humanizePluginID(pluginID: string) {
  const raw = String(pluginID || "").trim();
  const withoutPrefix = raw
    .replace(/^com\.powerx\.plugins\./, "")
    .replace(/^com\.powerx\.plugin\./, "")
    .replace(/^com\./, "");
  const withoutLocal = withoutPrefix.replace(/\.local$/, "");
  return humanizeModuleKey(withoutLocal || raw || "plugin");
}

function normalizeCapabilitySegment(value: string) {
  return String(value || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "") || "plugin";
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
  if (capability.source === "plugin") return humanizePluginCapabilityTitle(capability);
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
  if (!hasWorkflowCapabilityInvokeProtocol(capability)) return false;
  if (capability.source === "plugin") return true;
  return hasLocalizedCapabilityName(capability) || hasReadableCapabilityTitle(capability);
}

function looksLikeTechnicalCapabilityTitle(title: string) {
  return /^(GET|POST|PUT|PATCH|DELETE)\s+\/api\//i.test(title) || title === title.toLowerCase() && title.includes(".");
}

function humanizePluginCapabilityTitle(capability: RunCapabilityOption) {
  const capabilityID = String(capability.capabilityId || "").trim();
  const pluginID = String(capability.pluginId || "").trim();
  let businessPart = capabilityID;
  if (pluginID && capabilityID.startsWith(`${pluginID}.`)) {
    businessPart = capabilityID.slice(pluginID.length + 1);
  } else {
    businessPart = businessPart
      .replace(/^com\.powerx\.plugins\./, "")
      .replace(/^com\./, "");
  }
  return humanizeModuleKey(businessPart);
}

// 运行工作流
async function runWorkflow() {
  const definitionUUID = currentWorkflow.value?.uuid;
  if (!definitionUUID) {
    runError.value = t("workflow.errors.noCurrentDefinition");
    return;
  }
  if (!ensureWorkflowRuntimeConnected()) return;
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
  selectedRunCapabilitySource.value =
    capabilitySourceSelectItems.value.find((item) => item.value === (selectedCapability ? capabilitySourceKey(selectedCapability) : "")) ||
    capabilitySourceSelectItems.value[0] ||
    null;
  selectedRunCapabilityModule.value =
    runCapabilityModuleSelectItems.value.find((item) => item.value === (selectedCapability ? capabilityModuleOptionValue(selectedCapability) : "")) ||
    runCapabilityModuleSelectItems.value[0] ||
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
  if (!ensureWorkflowRuntimeConnected()) return;
  actingReviewTaskUUID.value = task.review_task_uuid;
  actingReviewAction.value = action;
  const previousRun = cloneRuntimeValue(latestRun.value);
  const previousReviewTasks = cloneRuntimeValue(latestReviewTasks.value);
  applyOptimisticReviewAction(task, action);
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
  } catch (err: any) {
    latestRun.value = previousRun;
    latestReviewTasks.value = previousReviewTasks || [];
    applyRunStateToCanvas();
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

function ensureWorkflowRuntimeConnected() {
  if (workflowRuntimeBus.connected.value) return true;
  toast.add({
    title: t("workflow.editor.runtimeDisconnected"),
    description: t("workflow.editor.runtimeRequiredForDebugRun"),
    color: "error",
  });
  return false;
}

function applyOptimisticReviewAction(task: HumanReviewTask, action: "approve" | "reject") {
  const run = latestRun.value;
  if (!run) return;
  const nextStepID = nextStepIDForReviewAction(task.step_id, action);
  const steps = [...(run.steps || [])];
  const reviewStepIndex = steps.findIndex((step) => step.step_id === task.step_id);
  if (reviewStepIndex >= 0) {
    steps[reviewStepIndex] = {
      ...steps[reviewStepIndex],
      state: "completed",
      awaiting_human: false,
      completed_at: new Date().toISOString(),
      payload_out: {
        ...(steps[reviewStepIndex].payload_out || {}),
        review: {
          action,
          workflow_instance_uuid: task.workflow_instance_uuid,
          step_id: task.step_id,
        },
      },
    };
  }
  if (nextStepID && !steps.some((step) => step.step_id === nextStepID)) {
    const nextNode = nodes.value.find((node) => node.id === nextStepID);
    steps.push({
      step_id: nextStepID,
      type: "system",
      node_kind: String(nextNode?.data?.kind || ""),
      state: "queued",
      scheduled_at: new Date().toISOString(),
      attempt: 1,
    });
  }
  latestRun.value = {
    ...run,
    state: nextStepID ? "running" : run.state,
    current_step_id: nextStepID || task.step_id,
    steps,
  };
  latestReviewTasks.value = latestReviewTasks.value.filter((item) => item.review_task_uuid !== task.review_task_uuid);
  applyRunStateToCanvas();
}

function nextStepIDForReviewAction(stepID: string, action: "approve" | "reject") {
  const handle = action === "approve" ? "approved" : "rejected";
  const edge = edges.value.find((item) => item.source === stepID && (item.sourceHandle || "out") === handle);
  return edge?.target || "";
}

function cloneRuntimeValue<T>(value: T): T {
  if (value === null || value === undefined) return value;
  return JSON.parse(JSON.stringify(value)) as T;
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

function effectiveNodeRunState(stepID: string) {
  if (!latestRun.value) return "";
  const step = latestRun.value.steps?.find((item) => item.step_id === stepID);
  if (step) return normalizeEffectiveStepState(step.state);
  if (isTerminalRunState(latestRunState.value)) return "skipped";
  return "pending";
}

function isTerminalRunState(state?: string) {
  return ["succeeded", "failed", "canceled"].includes(normalizeStepState(state));
}

function stepRunMessage(state: string, startedAt: string, completedAt: string, error: string) {
  if (error) return error;
  if (state === "skipped") return latestRunState.value === "canceled"
    ? t("workflow.editor.stepSkippedByCancel")
    : t("workflow.editor.stepSkippedByBranch");
  if (completedAt) return t("workflow.editor.stepCompletedAt", { time: completedAt });
  if (startedAt) return t("workflow.editor.stepStartedAt", { time: startedAt });
  return t("workflow.editor.notStarted");
}

function handleWorkflowRuntimeEvent(event: WorkflowRuntimeEvent) {
  runtimeEventQueue.push(event);
  void drainWorkflowRuntimeEventQueue();
}

async function drainWorkflowRuntimeEventQueue() {
  if (runtimeEventQueueProcessing) return;
  runtimeEventQueueProcessing = true;
  try {
    while (runtimeEventQueue.length > 0) {
      const event = runtimeEventQueue.shift();
      if (!event) continue;
      applyWorkflowRuntimeInstance(event.instance);
      await waitForRuntimeEventFrame(event);
    }
  } finally {
    runtimeEventQueueProcessing = false;
  }
}

async function waitForRuntimeEventFrame(event: WorkflowRuntimeEvent) {
  if (!isRuntimeStepTransitionEvent(event.event_type || "")) return;
  await new Promise((resolve) => window.setTimeout(resolve, 260));
}

function isRuntimeStepTransitionEvent(eventType: string) {
  return [
    "workflow.step.queued",
    "workflow.step.started",
    "workflow.step.waiting",
    "workflow.step.completed",
    "workflow.step.failed",
    "workflow.instance.running",
    "workflow.instance.succeeded",
    "workflow.instance.failed",
  ].includes(eventType);
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
  const nodeStates = new Map(nodes.value.map((node) => [node.id, effectiveNodeRunState(node.id)]));

  nodes.value = nodes.value.map((node) => ({
    ...node,
    data: {
      ...node.data,
      runState: nodeStates.get(node.id) || "",
    },
  }));

  edges.value = edges.value.map((edge) => {
    const runState = edgeRunState(nodeStates.get(edge.source), nodeStates.get(edge.target));
    return {
      ...edge,
      animated: runState === "active",
      class: runState ? `workflow-run-edge workflow-run-edge-${runState}` : "workflow-run-edge",
      data: {
        ...(edge.data || {}),
        runState,
      },
    };
  });
}

function edgeRunState(sourceState?: string, targetState?: string) {
  if (!latestRun.value) return "";
  if (sourceState === "skipped" || targetState === "skipped") return "skipped";
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
    class: "workflow-run-edge",
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
  if (node.data.kind === "capability.invoke") {
    void loadCapabilityReferenceData({ notify: false }).then(normalizeSelectedNodePreferredProtocol);
  }
  propertiesTab.value = "runtime";
}

function focusRuntimeStep(stepID: string) {
  if (!stepID) return;
  const node = findNode(stepID);
  if (!node) return;
  normalizeSelectedNodeProps(node);
  selectedNode.value = node;
  if (node.data.kind === "capability.invoke") {
    void loadCapabilityReferenceData({ notify: false }).then(normalizeSelectedNodePreferredProtocol);
  }
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
  edges.value = workflow.edges.map(workflowEdgeToVueFlowEdge);
  nodes.value = workflow.nodes.map((node) => workflowNodeToVueFlowNode(node));
  selectedNode.value = null;
  hasLoadedCanvas.value = true;
  applyRunStateToCanvas();
  nextTick(() => fitView({ padding: 0.28, duration: 0 }));
}

function workflowNodeToVueFlowNode(node: WfNode): Node {
  const ui = normalizeWorkflowNodeUi(node.kind, node.ui);
  const label = workflowNodeDisplayLabel(node);
  return {
    id: node.id,
    type: "generic",
    position: node.position,
    data: {
      kind: node.kind,
      paletteId: node.paletteId,
      label,
      props: node.props || {},
      ui,
      ports: kinds.value[node.kind]?.ports || workflowNodePorts(node.kind),
      schema: kinds.value[node.kind]?.schema || { type: "object", properties: {} },
    },
    width: ui.size?.w,
    height: ui.size?.h,
  };
}

function workflowNodeDisplayLabel(node: WfNode) {
  if (node.kind === "input.capture") return "workflow.node.start";
  if (node.kind === "workflow.end") return "workflow.node.end";
  return node.label;
}

function normalizeWorkflowNodeUi(kind: string, ui: WfNode["ui"]) {
  const visual = workflowNodeVisual(kind);
  return {
    ...ui,
    colorToken: visual.colorToken,
    icon: visual.icon,
    shape: workflowNodeShape(kind),
    size: {
      w: workflowNodeSize(kind, ui).w,
      h: workflowNodeSize(kind, ui).h,
    },
  };
}

function workflowNodePorts(kind: string) {
  if (kind === "input.capture") return { inputs: [], outputs: [{ name: "out" }] };
  if (kind === "workflow.end") return { inputs: [{ name: "in" }], outputs: [] };
  return {
    inputs: [{ name: "in" }],
    outputs: [{ name: "out" }, { name: "error" }],
  };
}

function workflowNodeShape(kind: string) {
  if (kind === "input.capture" || kind === "workflow.end") return "oval";
  return "card";
}

function workflowNodeSize(kind: string, ui: WfNode["ui"]) {
  if (kind === "input.capture" || kind === "workflow.end") return { w: 172, h: 76 };
  return { w: ui.size?.w || 240, h: ui.size?.h || 96 };
}

function workflowNodeVisual(kind: string) {
  if (kind === "input.capture") return { colorToken: "start", icon: "i-heroicons-play" };
  if (kind === "workflow.end") return { colorToken: "end", icon: "i-heroicons-stop" };
  if (kind.startsWith("skill.")) return { colorToken: "skill", icon: "i-heroicons-command-line" };
  if (kind.startsWith("capability.")) return { colorToken: "capability", icon: "i-heroicons-bolt" };
  if (kind.startsWith("knowledge.")) return { colorToken: "knowledge", icon: "i-heroicons-book-open" };
  if (kind.startsWith("metadata.")) return { colorToken: "metadata", icon: "i-heroicons-tag" };
  if (kind.startsWith("human.")) return { colorToken: "human", icon: "i-heroicons-user-circle" };
  if (kind.startsWith("decision.")) return { colorToken: "decision", icon: "i-heroicons-adjustments-horizontal" };
  if (kind.startsWith("parallel.")) return { colorToken: "parallel", icon: "i-heroicons-arrows-right-left" };
  if (kind.startsWith("event.")) return { colorToken: "event", icon: "i-heroicons-megaphone" };
  if (kind.startsWith("compensation.")) return { colorToken: "compensation", icon: "i-heroicons-arrow-uturn-left" };
  return { colorToken: "default", icon: "i-heroicons-square-3-stack-3d" };
}

function workflowEdgeToVueFlowEdge(edge: WorkflowEdge): Edge {
  return {
    id: edge.id,
    source: edge.source,
    sourceHandle: edge.sourceHandle || "out",
    target: edge.target,
    targetHandle: edge.targetHandle || "in",
    label: shouldRenderEdgeLabel(edge) ? edge.label : "",
    type: edge.type || "smoothstep",
    animated: false,
    class: "workflow-run-edge",
    labelStyle: {
      fill: "var(--wf-edge-label-text)",
      fontSize: "12px",
      fontWeight: 800,
    },
    labelBgStyle: {
      fill: "var(--wf-edge-label-bg)",
      fillOpacity: 1,
      stroke: "var(--wf-edge-label-border)",
      strokeWidth: 2,
    },
    labelBgPadding: [18, 9],
    labelBgBorderRadius: 8,
  };
}

function shouldRenderEdgeLabel(edge: WorkflowEdge) {
  const sourceHandle = String(edge.sourceHandle || "").trim();
  const label = String(edge.label || "").trim();
  return !["approved", "rejected"].includes(sourceHandle) && !["approved", "rejected"].includes(label);
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
    .replace(/[._-]+/g, " ")
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
  void loadCapabilityReferenceData({ notify: false });
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
  --wf-edge-label-bg: #f8fafc;
  --wf-edge-label-border: rgba(37, 99, 235, 0.62);
  --wf-edge-label-text: #0f172a;
  --workflow-palette-width: 248px;
  display: grid;
  grid-template-rows: 72px 34px auto minmax(0, 1fr) var(--workflow-bottom-height, 260px);
  height: 100dvh;
  max-height: 100dvh;
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
  --wf-edge-label-bg: #0f172a;
  --wf-edge-label-border: rgba(96, 165, 250, 0.74);
  --wf-edge-label-text: #f8fafc;
}

.workflow-topbar {
  grid-row: 1;
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
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  margin-top: 4px;
  color: var(--wf-muted);
  font-size: 12px;
}

.workflow-realtime-status {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 34px;
  max-width: 190px;
  min-width: 0;
  flex: none;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-panel-soft) 78%, var(--wf-panel));
  padding: 0 10px;
  color: var(--wf-muted);
  font-size: 12px;
  font-weight: 800;
}

.workflow-realtime-status span:last-of-type {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workflow-realtime-status :deep(svg) {
  width: 12px;
  height: 12px;
  flex: none;
}

.workflow-realtime-status.state-disconnected {
  border-color: color-mix(in srgb, #ef4444 38%, var(--wf-border));
  background: color-mix(in srgb, #ef4444 9%, var(--wf-panel));
  color: #f87171;
}

.workflow-realtime-status.state-connecting {
  border-color: color-mix(in srgb, #f59e0b 38%, var(--wf-border));
  background: color-mix(in srgb, #f59e0b 9%, var(--wf-panel));
  color: #fbbf24;
}

.workflow-realtime-status.state-connected {
  border-color: color-mix(in srgb, #22c55e 34%, var(--wf-border));
  background: color-mix(in srgb, #22c55e 9%, var(--wf-panel));
  color: #22c55e;
}

.workflow-realtime-dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 0 3px color-mix(in srgb, currentColor 14%, transparent);
}

.workflow-global-search {
  width: 100%;
}

.workflow-metrics {
  grid-row: 2;
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
  grid-row: 4;
  display: grid;
  grid-template-columns: var(--workflow-palette-width) minmax(0, 1fr) 420px;
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
  overflow: hidden;
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
  padding: 0 12px 32px;
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

.palette-item.disabled,
.palette-item.disabled:hover {
  cursor: not-allowed;
  opacity: 0.46;
  border-color: var(--wf-border);
  box-shadow: none;
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
  min-height: 0;
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

.workflow-structure-alert {
  grid-row: 3;
  margin: 10px 14px 0;
}

.workflow-properties {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
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
  min-height: 0;
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

.properties-field,
.properties-field :deep(.w-full),
.properties-field :deep(button),
.properties-field :deep(input),
.properties-field :deep(textarea) {
  width: 100%;
  min-width: 0;
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

.properties-note.state-error {
  border-color: color-mix(in srgb, #ef4444 46%, var(--wf-border));
  background: color-mix(in srgb, #ef4444 10%, var(--wf-panel));
  color: #ef4444;
}

.business-config-card {
  display: grid;
  gap: 6px;
  border: 1px solid color-mix(in srgb, var(--wf-accent) 32%, var(--wf-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-accent) 7%, var(--wf-panel));
  padding: 12px;
}

.business-config-card strong {
  color: var(--wf-text);
  font-size: 13px;
  font-weight: 800;
}

.business-config-card span {
  color: var(--wf-muted);
  font-size: 12px;
  line-height: 1.5;
}

.business-config-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.business-config-item {
  min-width: 0;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 9px 10px;
}

.business-config-item span {
  display: block;
  margin-bottom: 5px;
  color: var(--wf-muted);
  font-size: 11px;
  line-height: 1.25;
}

.business-config-item strong {
  display: block;
  overflow-wrap: anywhere;
  color: var(--wf-text);
  font-size: 12px;
  font-weight: 750;
  line-height: 1.35;
}

.readonly-business-value {
  min-height: 34px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 8px 10px;
  color: var(--wf-text);
  font-size: 13px;
  font-weight: 700;
  line-height: 1.35;
}

.advanced-config-section {
  display: grid;
  gap: 10px;
  margin-top: 4px;
  border-top: 1px solid var(--wf-border);
  padding-top: 12px;
}

.advanced-config-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel-soft);
  padding: 8px 10px;
  color: var(--wf-text);
  font-size: 12px;
  font-weight: 800;
}

.advanced-config-toggle:hover {
  border-color: color-mix(in srgb, var(--wf-accent) 38%, var(--wf-border));
  color: var(--wf-accent);
}

.advanced-config-list {
  display: grid;
  gap: 8px;
}

.advanced-config-row {
  display: grid;
  gap: 5px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-panel-soft) 72%, var(--wf-panel));
  padding: 8px 10px;
}

.advanced-config-row span {
  color: var(--wf-muted);
  font-size: 11px;
  font-weight: 700;
}

.advanced-config-row code {
  max-height: 120px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--wf-text);
  font-size: 11px;
  line-height: 1.45;
}

.advanced-config-list p {
  color: var(--wf-dim);
  font-size: 11px;
  line-height: 1.5;
}

.node-runtime-panel {
  display: grid;
  gap: 14px;
}

.node-runtime-summary,
.runtime-review-card,
.runtime-summary-card,
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

.runtime-summary-card {
  display: grid;
  gap: 10px;
}

.runtime-summary-card > span {
  color: var(--wf-muted);
  font-size: 12px;
  font-weight: 700;
}

.runtime-summary-list {
  display: grid;
  gap: 8px;
}

.runtime-summary-row {
  display: grid;
  grid-template-columns: minmax(92px, 0.42fr) minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  border-radius: 7px;
  background: color-mix(in srgb, var(--wf-panel) 58%, transparent);
  padding: 8px 10px;
}

.runtime-summary-row span {
  color: var(--wf-dim);
  font-size: 11px;
  font-weight: 700;
}

.runtime-summary-row strong {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--wf-text);
  font-size: 12px;
  line-height: 1.45;
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
  grid-row: 5;
  position: relative;
  z-index: 20;
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
  position: fixed;
  z-index: 30;
  left: calc(var(--workflow-palette-width) + 18px);
  bottom: calc(var(--workflow-bottom-height, 260px) + 16px);
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

:deep(.workflow-run-edge .vue-flow__edge-text) {
  paint-order: stroke;
  stroke: var(--wf-edge-label-bg);
  stroke-width: 6px;
  fill: var(--wf-edge-label-text) !important;
  pointer-events: none;
}

:deep(.workflow-run-edge .vue-flow__edge-textbg) {
  rx: 4;
  ry: 4;
  fill: var(--wf-edge-label-bg) !important;
  fill-opacity: 1 !important;
  stroke: var(--wf-edge-label-border) !important;
  stroke-width: 2px !important;
  pointer-events: none;
}

:deep(.workflow-run-edge-pending .vue-flow__edge-path) {
  stroke: rgba(148, 163, 184, 0.42);
  opacity: 0.55;
}

:deep(.workflow-run-edge-skipped .vue-flow__edge-path) {
  stroke: rgba(148, 163, 184, 0.5);
  opacity: 0.45;
  stroke-dasharray: 6 5;
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
  position: fixed;
  z-index: 31;
  left: calc(var(--workflow-palette-width) + 206px);
  bottom: calc(var(--workflow-bottom-height, 260px) + 16px);
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

.workflow-editor.dark :deep(.wf-node-start) {
  background: linear-gradient(180deg, rgba(20, 184, 166, 0.16), #131d2b) !important;
  border-color: #14b8a6 !important;
}

.workflow-editor.dark :deep(.wf-node-end) {
  background: linear-gradient(180deg, rgba(148, 163, 184, 0.16), #131d2b) !important;
  border-color: #94a3b8 !important;
}

.workflow-editor.dark :deep(.wf-node-skill) {
  background: linear-gradient(180deg, rgba(139, 92, 246, 0.18), #131d2b) !important;
  border-color: #8b5cf6 !important;
}

.workflow-editor.dark :deep(.wf-node-capability) {
  background: linear-gradient(180deg, rgba(37, 99, 235, 0.18), #131d2b) !important;
  border-color: #3b82f6 !important;
}

.workflow-editor.dark :deep(.wf-node-metadata) {
  background: linear-gradient(180deg, rgba(6, 182, 212, 0.18), #131d2b) !important;
  border-color: #06b6d4 !important;
}

.workflow-editor.dark :deep(.wf-node-knowledge) {
  background: linear-gradient(180deg, rgba(34, 197, 94, 0.16), #131d2b) !important;
  border-color: #22c55e !important;
}

.workflow-editor.dark :deep(.wf-node-human),
.workflow-editor.dark :deep(.wf-node-decision),
.workflow-editor.dark :deep(.wf-node-parallel) {
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.18), #131d2b) !important;
  border-color: #f59e0b !important;
}

.workflow-editor.dark :deep(.wf-node-event) {
  background: linear-gradient(180deg, rgba(14, 165, 233, 0.18), #131d2b) !important;
  border-color: #0ea5e9 !important;
}

.workflow-editor.dark :deep(.wf-node-compensation) {
  background: linear-gradient(180deg, rgba(239, 68, 68, 0.18), #131d2b) !important;
  border-color: #ef4444 !important;
}

.workflow-editor.dark :deep(.wf-node.selected) {
  border-color: #22c55e !important;
  box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.24), 0 0 26px rgba(34, 197, 94, 0.34), 0 14px 30px rgba(0, 0, 0, 0.34) !important;
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
