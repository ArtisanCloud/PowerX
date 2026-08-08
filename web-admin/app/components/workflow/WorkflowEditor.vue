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
                <div class="input-fields-editor">
                  <div class="input-fields-header">
                    <div>
                      <strong>{{ t("workflow.editor.inputFieldsTitle") }}</strong>
                      <span>{{ t("workflow.editor.inputFieldsDescription") }}</span>
                    </div>
                    <UButton
                      size="xs"
                      color="primary"
                      variant="soft"
                      icon="i-heroicons-plus"
                      :aria-label="t('workflow.editor.addInputField')"
                      @click="addInputRunFormField"
                    >
                      {{ t("workflow.editor.addInputFieldShort") }}
                    </UButton>
                  </div>
                  <div v-if="selectedInputRunFormFields.length" class="input-field-list">
                    <div
                      v-for="(field, fieldIndex) in selectedInputRunFormFields"
                      :key="field.key || fieldIndex"
                      class="input-field-list-item"
                    >
                      <div class="input-field-list-head">
                        <strong>{{ inputFieldDisplayLabel(field, fieldIndex) }}</strong>
                        <div class="input-field-list-actions">
                          <UButton
                            size="xs"
                            color="neutral"
                            variant="ghost"
                            icon="i-heroicons-pencil-square"
                            :aria-label="t('workflow.editor.editInputField')"
                            @click="openInputFieldDialog(fieldIndex)"
                          />
                          <UButton
                            size="xs"
                            color="error"
                            variant="ghost"
                            icon="i-heroicons-trash"
                            :aria-label="t('workflow.editor.removeInputField')"
                            @click="removeInputRunFormField(fieldIndex)"
                          />
                        </div>
                      </div>
                      <div class="input-field-list-badges">
                        <UBadge class="input-field-badge" color="neutral" variant="subtle">
                          {{ inputFieldKindLabel(field.kind) }}
                        </UBadge>
                        <UBadge v-if="field.required" class="input-field-badge" color="error" variant="subtle">
                          {{ t("workflow.editor.inputFieldRequiredLabel") }}
                        </UBadge>
                        <UBadge
                          v-if="field.resource === 'knowledge_space'"
                          class="input-field-badge"
                          color="primary"
                          variant="subtle"
                        >
                          {{ t("workflow.editor.inputFieldKnowledgeSpaceResourceLabel") }}
                        </UBadge>
                      </div>
                    </div>
                  </div>
                  <div v-else class="input-field-empty">
                    {{ t("workflow.editor.inputFieldsEmpty") }}
                  </div>
                </div>
              </template>

              <template v-else-if="selectedNode.data.kind === 'skill.invoke'">
                <div class="business-config-card">
                  <strong>{{ t("workflow.editor.skillBusinessTitle") }}</strong>
                  <span>{{ t("workflow.editor.skillBusinessDescription") }}</span>
                </div>
                <UFormField class="properties-field" :label="t('workflow.editor.skillNodeSkillLabel')">
                  <USelectMenu
                    v-model="selectedNodeSkill"
                    :items="skillSelectItems"
                    label-key="label"
                    :portal="false"
                    :content="runDialogSelectContent"
                    :ui="runDialogSelectUi"
                    class="w-full"
                    :loading="skillsLoading"
                    :placeholder="t('workflow.editor.skillSelectPlaceholder')"
                    :search-input="{ placeholder: t('workflow.editor.skillSearchPlaceholder') }"
                    :disabled="skillsLoading || skillSelectItems.length === 0"
                  />
                  <div v-if="skillsError" class="properties-note state-error">
                    {{ skillsError }}
                  </div>
                  <div v-else-if="selectedNodeSkillRecord" class="properties-note">
                    {{ selectedNodeSkillDetail }}
                  </div>
                </UFormField>
                <div class="skill-model-grid">
                  <UFormField class="properties-field" :label="t('workflow.editor.skillModelModalityLabel')">
                    <USelect
                      v-model="selectedNodeSkillModelModality"
                      :items="skillModelModalityOptions"
                      value-key="value"
                      label-key="label"
                      class="w-full"
                    />
                  </UFormField>
                  <UFormField class="properties-field" :label="t('workflow.editor.skillModelProfileLabel')">
                    <USelectMenu
                      v-model="selectedNodeSkillModelProfile"
                      :items="selectedNodeSkillModelProfileItems"
                      label-key="label"
                      :portal="false"
                      :content="runDialogSelectContent"
                      :ui="runDialogSelectUi"
                      class="w-full"
                      :loading="modelProfilesLoading"
                      :placeholder="t('workflow.editor.skillModelProfilePlaceholder')"
                      :search-input="{ placeholder: t('workflow.editor.skillModelProfileSearchPlaceholder') }"
                      :disabled="modelProfilesLoading || selectedNodeSkillModelProfileItems.length === 0"
                    />
                  </UFormField>
                </div>
                <div v-if="modelProfilesError" class="properties-note state-error">
                  {{ modelProfilesError }}
                </div>
                <div v-else class="properties-note">
                  {{ t("workflow.editor.skillModelOverrideHint") }}
                </div>
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
                <template v-if="selectedNode.data.kind === 'metadata.classify'">
                  <UFormField class="properties-field" :label="t('workflow.editor.metadataStrategyLabel')">
                    <USelect
                      v-model="selectedNodeMetadataStrategy"
                      :items="metadataStrategyOptions"
                      value-key="value"
                      label-key="label"
                      class="w-full"
                    />
                  </UFormField>
                  <UFormField class="properties-field" :label="t('workflow.fields.taxonomy_namespace')">
                    <USelectMenu
                      v-model="selectedNodeTaxonomy"
                      :items="metadataTaxonomySelectItems"
                      label-key="label"
                      :portal="false"
                      :content="runDialogSelectContent"
                      :ui="runDialogSelectUi"
                      class="w-full"
                      :loading="metadataOptionsLoading"
                      :placeholder="t('workflow.editor.metadataTaxonomyPlaceholder')"
                      :search-input="{ placeholder: t('workflow.editor.metadataTaxonomySearchPlaceholder') }"
                      :disabled="metadataOptionsLoading || metadataTaxonomySelectItems.length === 0"
                    />
                  </UFormField>
                  <UFormField class="properties-field" :label="t('workflow.fields.tag_namespace')">
                    <USelectMenu
                      v-model="selectedNodeTagNamespace"
                      :items="metadataTagNamespaceSelectItems"
                      label-key="label"
                      :portal="false"
                      :content="runDialogSelectContent"
                      :ui="runDialogSelectUi"
                      class="w-full"
                      :loading="metadataOptionsLoading"
                      :placeholder="t('workflow.editor.metadataTagNamespacePlaceholder')"
                      :search-input="{ placeholder: t('workflow.editor.metadataTagNamespaceSearchPlaceholder') }"
                      :disabled="metadataOptionsLoading || metadataTagNamespaceSelectItems.length === 0"
                    />
                  </UFormField>
                  <UFormField class="properties-field" :label="t('workflow.fields.dictionary_namespace')">
                    <USelectMenu
                      v-model="selectedNodeDictionary"
                      :items="metadataDictionarySelectItems"
                      label-key="label"
                      :portal="false"
                      :content="runDialogSelectContent"
                      :ui="runDialogSelectUi"
                      class="w-full"
                      :loading="metadataOptionsLoading"
                      :placeholder="t('workflow.editor.metadataDictionaryPlaceholder')"
                      :search-input="{ placeholder: t('workflow.editor.metadataDictionarySearchPlaceholder') }"
                      :disabled="metadataOptionsLoading || metadataDictionarySelectItems.length === 0"
                    />
                  </UFormField>
                  <UFormField class="properties-field" :label="t('workflow.fields.resource_type_namespace')">
                    <USelectMenu
                      v-model="selectedNodeResourceType"
                      :items="metadataResourceTypeSelectItems"
                      label-key="label"
                      :portal="false"
                      :content="runDialogSelectContent"
                      :ui="runDialogSelectUi"
                      class="w-full"
                      :loading="metadataOptionsLoading"
                      :placeholder="t('workflow.editor.metadataResourceTypePlaceholder')"
                      :search-input="{ placeholder: t('workflow.editor.metadataResourceTypeSearchPlaceholder') }"
                      :disabled="metadataOptionsLoading || metadataResourceTypeSelectItems.length === 0"
                    />
                  </UFormField>
                  <div v-if="metadataOptionsError" class="properties-note state-error">
                    {{ metadataOptionsError }}
                  </div>
                  <div v-else class="properties-note">
                    {{ t("workflow.editor.metadataConfigHint") }}
                  </div>
                </template>
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
          <div v-if="runFormFields.length > 0" class="debug-input-form">
            <UAlert
              class="run-dialog-notice"
              icon="i-heroicons-information-circle"
              color="info"
              variant="subtle"
              :title="runFormTitle"
              :description="runFormDescription"
            />
            <div class="run-start-node-card">
              <div class="run-start-node-icon">
                <UIcon name="i-heroicons-play" />
              </div>
              <div class="run-start-node-content">
                <span>{{ t("workflow.editor.runFormStartNodeLabel") }}</span>
                <strong>{{ runFormStartNodeName }}</strong>
              </div>
              <div class="run-start-node-meta">
                <UBadge color="primary" variant="subtle">
                  {{ currentInputSchemaRef || t("workflow.editor.inlineInputSchema") }}
                </UBadge>
                <UBadge color="neutral" variant="subtle">
                  {{ t("workflow.editor.runFormFieldCount", { count: runFormFields.length }) }}
                </UBadge>
              </div>
            </div>
            <template v-for="field in visibleRunFormFields" :key="field.key">
              <UFormField :label="runFormFieldLabel(field)" :required="field.required">
                <USelectMenu
                  v-if="field.resource === 'knowledge_space'"
                  :model-value="genericSelectValue(field)"
                  :items="marketingKnowledgeSpaceSelectItems"
                  label-key="label"
                  :portal="runDialogSelectPortal"
                  :content="runDialogSelectContent"
                  :ui="runDialogSelectUi"
                  class="w-full"
                  :loading="knowledgeSpacesLoading"
                  :disabled="knowledgeSpacesLoading || marketingKnowledgeSpaceSelectItems.length === 0"
                  :placeholder="runFormFieldPlaceholder(field)"
                  :search-input="{ placeholder: t('workflow.editor.marketingKnowledgeSpaceSearchPlaceholder') }"
                  @update:model-value="setGenericSelectValue(field, $event as SelectOption | null)"
                />
                <USelectMenu
                  v-else-if="field.resource === 'capability'"
                  :model-value="genericSelectValue(field)"
                  :items="genericCapabilitySelectItems"
                  label-key="label"
                  :portal="runDialogSelectPortal"
                  :content="runDialogSelectContent"
                  :ui="runDialogSelectUi"
                  class="w-full"
                  :loading="capabilityOptionsLoading"
                  :disabled="capabilityOptionsLoading || genericCapabilitySelectItems.length === 0"
                  :placeholder="runFormFieldPlaceholder(field)"
                  :search-input="{ placeholder: t('workflow.editor.capabilitySearchPlaceholder') }"
                  @update:model-value="setGenericSelectValue(field, $event as SelectOption | null)"
                />
                <USelect
                  v-else-if="field.kind === 'select'"
                  v-model="runFormValues[field.key]"
                  :items="runFormFieldOptions(field)"
                  value-key="value"
                  label-key="label"
                  class="w-full"
                />
                <UTextarea
                  v-else-if="field.kind === 'textarea' || field.kind === 'object'"
                  v-model="runFormValues[field.key]"
                  :placeholder="runFormFieldPlaceholder(field)"
                  :rows="field.rows || (field.kind === 'object' ? 8 : 4)"
                  class="w-full"
                />
                <label v-else-if="field.kind === 'boolean'" class="debug-input-switch">
                  <USwitch v-model="runFormValues[field.key]" />
                  <span>
                    <strong>{{ runFormFieldLabel(field) }}</strong>
                    <small v-if="runFormFieldHint(field)">{{ runFormFieldHint(field) }}</small>
                  </span>
                </label>
                <UInput
                  v-else
                  v-model="runFormValues[field.key]"
                  :type="field.kind === 'number' ? 'number' : 'text'"
                  :placeholder="runFormFieldPlaceholder(field)"
                  class="w-full"
                />
                <div v-if="runFormFieldHint(field) && field.kind !== 'boolean'" class="run-field-hint">
                  {{ runFormFieldHint(field) }}
                </div>
                <div v-if="field.resource === 'knowledge_space' && knowledgeSpacesError" class="run-field-hint state-error">
                  {{ knowledgeSpacesError }}
                </div>
                <div v-if="field.resource === 'capability' && capabilityOptionsError" class="run-field-hint state-error">
                  {{ capabilityOptionsError }}
                </div>
              </UFormField>
            </template>
          </div>
          <div v-else-if="runFormConfigError" class="debug-input-form">
            <UAlert
              class="run-dialog-notice"
              icon="i-heroicons-exclamation-triangle"
              color="error"
              variant="subtle"
              :title="t('workflow.editor.runFormConfigErrorTitle')"
              :description="runFormConfigError"
            />
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
            :disabled="capabilityOptionsLoading || Boolean(runFormConfigError)"
            @click="runWorkflow"
          >
            {{ t("workflow.editor.startRun") }}
          </UButton>
        </div>
      </template>
    </UModal>

    <UModal
      v-model:open="inputFieldDialogOpen"
      :title="inputFieldDialogTitle"
      :description="t('workflow.editor.inputFieldDialogDescription')"
      :ui="{ content: 'input-field-dialog max-w-4xl w-[88vw]', body: 'w-full', footer: 'w-full' }"
    >
      <template #body>
        <div class="input-field-dialog-form">
          <section class="input-field-editor-section">
            <div class="input-field-editor-section-title">
              <strong>{{ t("workflow.editor.inputFieldBasicSection") }}</strong>
            </div>
            <div class="input-field-grid">
              <UFormField :label="t('workflow.editor.inputFieldLabelLabel')" required>
                <UInput
                  v-model="inputFieldDraft.label"
                  :placeholder="t('workflow.editor.inputFieldLabelPlaceholder')"
                />
              </UFormField>
              <UFormField :label="t('workflow.editor.inputFieldKindLabel')" required>
                <USelect
                  v-model="inputFieldDraft.kind"
                  :items="inputFieldKindOptions"
                  value-key="value"
                  label-key="label"
                  class="w-full"
                  @update:model-value="handleInputFieldKindChange"
                />
              </UFormField>
            </div>
          </section>
          <section class="input-field-editor-section">
            <div class="input-field-editor-section-title">
              <strong>{{ t("workflow.editor.inputFieldUiSection") }}</strong>
            </div>
            <UFormField class="properties-field" :label="t('workflow.editor.inputFieldPlaceholderLabel')">
              <UInput
                v-model="inputFieldDraft.placeholder"
                :placeholder="t('workflow.editor.inputFieldPlaceholderPlaceholder')"
              />
            </UFormField>
            <UFormField
              v-if="inputFieldDraft.kind === 'select' && !inputFieldDraft.knowledgeSpaceResource"
              class="properties-field"
              :label="t('workflow.editor.inputFieldOptionsLabel')"
            >
              <div class="input-field-option-grid">
                <label
                  v-for="option in inputFieldDraft.options"
                  :key="option.value"
                  class="input-field-option-choice"
                >
                  <UCheckbox v-model="option.selected" />
                  <span>{{ option.label }}</span>
                </label>
              </div>
            </UFormField>
          </section>
          <section class="input-field-editor-section">
            <div class="input-field-editor-section-title">
              <strong>{{ t("workflow.editor.inputFieldBehaviorSection") }}</strong>
            </div>
            <div class="input-field-flags">
              <label>
                <USwitch v-model="inputFieldDraft.required" />
                <span>{{ t("workflow.editor.inputFieldRequiredLabel") }}</span>
              </label>
              <label>
                <USwitch
                  v-model="inputFieldDraft.knowledgeSpaceResource"
                  @update:model-value="handleInputFieldKnowledgeSpaceResourceChange"
                />
                <span>{{ t("workflow.editor.inputFieldKnowledgeSpaceResourceLabel") }}</span>
              </label>
            </div>
          </section>
        </div>
      </template>
      <template #footer>
        <div class="input-field-dialog-footer">
          <UButton color="neutral" variant="subtle" @click="closeInputFieldDialog">
            {{ t("common.cancel") }}
          </UButton>
          <UButton color="primary" icon="i-heroicons-check" @click="saveInputFieldDialog">
            {{ t("common.save") }}
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
import { AISettingService, useMetadataGovernanceService, useSkillsService, type AgentProfile, type SkillRecord } from "~/composables/api/services";
import { useKnowledgeSpaces, type KnowledgeSpaceRecord } from "~/composables/useKnowledgeSpaces";
import type { DictionaryNamespace, MetadataTag, ResourceType, Taxonomy } from "~/types/metadata-governance";

// 主题支持
const colorMode = useColorMode();
const isDark = computed(() => colorMode.value === "dark");
const { t, te } = useI18n();
const route = useRoute();
const toast = useToast();
const workflowService = useWorkflowService();
const workflowRuntimeBus = useWorkflowRuntimeBus();
const knowledgeSpacesApi = useKnowledgeSpaces();
const skillsService = useSkillsService();
const metadataGovernanceService = useMetadataGovernanceService();

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
type RunFormFieldKind = "text" | "textarea" | "select" | "boolean" | "number" | "object";
type RunFormField = {
  key: string;
  path: string;
  kind: RunFormFieldKind;
  labelKey?: string;
  label?: string;
  placeholderKey?: string;
  placeholder?: string;
  hintKey?: string;
  hint?: string;
  required?: boolean;
  rows?: number;
  defaultValue?: any;
  options?: Array<SelectOption & { labelKey?: string }>;
  resource?: "knowledge_space" | "capability";
  visibleWhen?: { field: string; equals?: string | string[]; notEquals?: string | string[] };
};
type RunFormDefinition = {
  titleKey?: string;
  title?: string;
  descriptionKey?: string;
  description?: string;
  fields: RunFormField[];
};
type InputFieldOptionDraft = SelectOption & {
  selected: boolean;
};
type InputFieldDraft = {
  key: string;
  path: string;
  kind: RunFormFieldKind;
  label: string;
  placeholder: string;
  required: boolean;
  options: InputFieldOptionDraft[];
  knowledgeSpaceResource: boolean;
};
type SkillSelectOption = SelectOption & {
  skillID: string;
  version: string;
};
type ModelProfileSelectOption = SelectOption & {
  modality: string;
  provider: string;
  model: string;
};

const capabilityOptions = ref<RunCapabilityOption[]>([]);
const capabilityOptionsLoading = ref(false);
const capabilityOptionsError = ref("");
const knowledgeSpaces = ref<KnowledgeSpaceRecord[]>([]);
const knowledgeSpacesLoading = ref(false);
const knowledgeSpacesError = ref("");
const runFormValues = reactive<Record<string, any>>({});
const runFormSelectValues = ref<Record<string, SelectOption | null>>({});
const inputFieldDialogOpen = ref(false);
const editingInputFieldIndex = ref<number | null>(null);
const inputFieldDraft = reactive<InputFieldDraft>({
  key: "",
  path: "",
  kind: "text",
  label: "",
  placeholder: "",
  required: false,
  options: [],
  knowledgeSpaceResource: false,
});
const skillRecords = ref<SkillRecord[]>([]);
const skillsLoading = ref(false);
const skillsError = ref("");
const modelProfiles = ref<AgentProfile[]>([]);
const modelProfilesLoading = ref(false);
const modelProfilesError = ref("");
const metadataTaxonomies = ref<Taxonomy[]>([]);
const metadataDictionaries = ref<DictionaryNamespace[]>([]);
const metadataTags = ref<MetadataTag[]>([]);
const metadataResourceTypes = ref<ResourceType[]>([]);
const metadataOptionsLoading = ref(false);
const metadataOptionsError = ref("");
const actingReviewTaskUUID = ref("");
const actingReviewAction = ref("");
const bottomPanelHeight = ref(260);
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

const marketingKnowledgeSpaceSelectItems = computed<SelectOption[]>(() =>
  knowledgeSpaces.value.map((space) => ({
    label: knowledgeSpaceOptionLabel(space),
    value: space.spaceId,
  }))
);

const genericCapabilitySelectItems = computed<SelectOption[]>(() =>
  workflowRunnableCapabilities.value
    .sort((left, right) => Number(hasLocalizedCapabilityName(right)) - Number(hasLocalizedCapabilityName(left)))
    .map((capability) => ({
      label: capabilityOptionLabel(capability),
      value: capability.capabilityId,
    }))
);

const currentInputCaptureStep = computed(() =>
  (currentWorkflow.value?.raw?.step_graph || []).find((step) => step.node_kind === "input.capture") || null
);

const currentInputCaptureNode = computed(() =>
  nodes.value.find((node) => node.data?.kind === "input.capture") || null
);

const currentInputSchemaRef = computed(() =>
  String(
    currentInputCaptureNode.value?.data?.props?.input_schema_ref ||
    currentInputCaptureStep.value?.config?.input_schema_ref ||
    currentWorkflow.value?.raw?.metadata?.input_schema_ref ||
    ""
  ).trim()
);

const runFormDefinition = computed<RunFormDefinition | null>(() => {
  const fromNodeSchema = runFormDefinitionFromSchema(currentInputCaptureNode.value?.data?.props?.input_schema);
  if (fromNodeSchema) return fromNodeSchema;
  const fromWorkflowSchema = runFormDefinitionFromSchema(currentWorkflow.value?.raw?.input_schema);
  if (fromWorkflowSchema) return fromWorkflowSchema;
  const fromStepSchema = runFormDefinitionFromSchema(currentInputCaptureStep.value?.config?.input_schema);
  if (fromStepSchema) return fromStepSchema;
  const schema = currentWorkflow.value?.raw?.input_schema;
  const fromSchema = runFormDefinitionFromSchema(schema);
  if (fromSchema) return fromSchema;
  return builtinRunFormDefinitions[currentInputSchemaRef.value] || null;
});

const runFormFields = computed<RunFormField[]>(() => runFormDefinition.value?.fields || []);
const visibleRunFormFields = computed(() => runFormFields.value.filter(isRunFormFieldVisible));
const runFormConfigError = computed(() => {
  if (!currentInputSchemaRef.value || runFormDefinition.value) return "";
  return t("workflow.editor.runFormSchemaRefMissing", { ref: currentInputSchemaRef.value });
});
const runFormStartNodeName = computed(() => {
  const stepID = String(currentInputCaptureNode.value?.id || currentInputCaptureStep.value?.id || "").trim();
  if (!stepID) return t("workflow.editor.notConfigured");
  return stepDisplayName(stepID);
});
const runFormTitle = computed(() => {
  const form = runFormDefinition.value;
  if (!form) return t("workflow.editor.startNodeRunFormTitle");
  const title = form.titleKey && te(form.titleKey) ? t(form.titleKey) : form.title || t("workflow.editor.startNodeRunFormTitle");
  return t("workflow.editor.startNodeRunFormTitleWithName", { name: title });
});
const runFormDescription = computed(() => {
  const form = runFormDefinition.value;
  if (!form) return t("workflow.editor.startNodeRunFormDescription");
  if (form.descriptionKey && te(form.descriptionKey)) return t(form.descriptionKey);
  return form.description || t("workflow.editor.startNodeRunFormDescription");
});

const inputFieldKindOptions = computed<SelectOption[]>(() => [
  { label: t("workflow.inputFieldKind.text"), value: "text" },
  { label: t("workflow.inputFieldKind.textarea"), value: "textarea" },
  { label: t("workflow.inputFieldKind.select"), value: "select" },
  { label: t("workflow.inputFieldKind.boolean"), value: "boolean" },
  { label: t("workflow.inputFieldKind.number"), value: "number" },
  { label: t("workflow.inputFieldKind.object"), value: "object" },
]);

const selectedInputRunFormFields = computed<RunFormField[]>(() => {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "input.capture") return [];
  return editableInputRunFormFields(node);
});

const inputFieldDialogTitle = computed(() =>
  editingInputFieldIndex.value === null ? t("workflow.editor.addInputField") : t("workflow.editor.editInputField")
);

const builtinRunFormDefinitions: Record<string, RunFormDefinition> = {
  "workflow.input.approval_guarded_capability.v1": {
    titleKey: "workflow.editor.startNodeRunFormTitle",
    descriptionKey: "workflow.editor.startNodeRunFormDescription",
    fields: [
      {
        key: "capability_id",
        path: "capability_id",
        kind: "select",
        resource: "capability",
        required: true,
        labelKey: "workflow.editor.runCapabilityLabel",
        placeholderKey: "workflow.editor.capabilitySelectPlaceholder",
        hintKey: "workflow.editor.capabilitySelectHint",
        defaultValue: "com.corex.metadata.dictionary.read",
      },
      {
        key: "reason",
        path: "request.reason",
        kind: "select",
        required: true,
        labelKey: "workflow.fields.reason",
        placeholderKey: "workflow.editor.executionReasonPlaceholder",
        hintKey: "workflow.editor.executionReasonHint",
        defaultValue: "workflow_debug_approval_guarded_capability",
        options: [
          { labelKey: "workflow.executionReason.workflowDebugApprovalGuardedCapability", label: "", value: "workflow_debug_approval_guarded_capability" },
          { labelKey: "workflow.executionReason.permissionBoundaryTest", label: "", value: "permission_boundary_test" },
          { labelKey: "workflow.executionReason.businessDryRun", label: "", value: "business_dry_run" },
        ],
      },
      {
        key: "dry_run",
        path: "request.payload.dry_run",
        kind: "boolean",
        labelKey: "workflow.fields.dry_run",
        hintKey: "workflow.editor.dryRunHint",
        defaultValue: true,
      },
      {
        key: "note",
        path: "request.payload.note",
        kind: "textarea",
        rows: 3,
        labelKey: "workflow.fields.note",
        placeholderKey: "workflow.editor.debugNotePlaceholder",
        defaultValue: "",
      },
    ],
  },
  "workflow.input.marketing_source.v1": {
    titleKey: "workflow.editor.marketingRunFormTitle",
    descriptionKey: "workflow.editor.startNodeRunFormDescription",
    fields: [
      {
        key: "knowledge_space_uuid",
        path: "knowledge_space_uuid",
        kind: "select",
        resource: "knowledge_space",
        required: true,
        labelKey: "workflow.editor.marketingKnowledgeSpaceLabel",
        placeholderKey: "workflow.editor.marketingKnowledgeSpacePlaceholder",
        hintKey: "workflow.editor.marketingKnowledgeSpaceHint",
      },
      {
        key: "source_type",
        path: "source.type",
        kind: "select",
        required: true,
        labelKey: "workflow.editor.marketingSourceTypeLabel",
        defaultValue: "text",
        options: [
          { labelKey: "workflow.marketingInput.sourceType.text", label: "", value: "text" },
          { labelKey: "workflow.marketingInput.sourceType.audio", label: "", value: "audio" },
          { labelKey: "workflow.marketingInput.sourceType.document", label: "", value: "document" },
          { labelKey: "workflow.marketingInput.sourceType.link", label: "", value: "link" },
        ],
      },
      {
        key: "text",
        path: "source.content",
        kind: "textarea",
        rows: 7,
        required: true,
        labelKey: "workflow.editor.marketingSourceTextLabel",
        placeholderKey: "workflow.editor.marketingSourceTextPlaceholder",
        defaultValue: t("workflow.editor.marketingSourceTextExample"),
        visibleWhen: { field: "source_type", equals: "text" },
      },
      {
        key: "url",
        path: "source.url",
        kind: "text",
        required: true,
        labelKey: "workflow.editor.marketingSourceUrlLabel",
        placeholderKey: "workflow.editor.marketingSourceUrlPlaceholder",
        defaultValue: "",
        visibleWhen: { field: "source_type", equals: "link" },
      },
      {
        key: "asset_uuid",
        path: "source.asset_uuid",
        kind: "text",
        required: true,
        labelKey: "workflow.editor.marketingSourceAssetLabel",
        placeholderKey: "workflow.editor.marketingSourceAssetPlaceholder",
        hintKey: "workflow.editor.marketingSourceAssetHint",
        defaultValue: "",
        visibleWhen: { field: "source_type", equals: ["audio", "document"] },
      },
      {
        key: "context",
        path: "source.context",
        kind: "text",
        labelKey: "workflow.editor.marketingContextLabel",
        placeholderKey: "workflow.editor.marketingContextPlaceholder",
        defaultValue: t("workflow.editor.marketingContextDefault"),
      },
      {
        key: "language",
        path: "source.language",
        kind: "select",
        labelKey: "workflow.editor.marketingLanguageLabel",
        defaultValue: "zh",
        options: [
          { labelKey: "workflow.marketingInput.language.zh", label: "", value: "zh" },
          { labelKey: "workflow.marketingInput.language.en", label: "", value: "en" },
          { labelKey: "workflow.marketingInput.language.ja", label: "", value: "ja" },
          { labelKey: "workflow.marketingInput.language.ko", label: "", value: "ko" },
        ],
      },
      {
        key: "note",
        path: "note",
        kind: "textarea",
        rows: 3,
        labelKey: "workflow.editor.marketingRunNoteLabel",
        placeholderKey: "workflow.editor.marketingRunNotePlaceholder",
        defaultValue: "",
      },
    ],
  },
};

builtinRunFormDefinitions["workflow.input.knowledge_source.v1"] = {
  titleKey: "workflow.editor.knowledgeSourceRunFormTitle",
  descriptionKey: "workflow.editor.knowledgeSourceRunFormDescription",
  fields: [
    {
      key: "knowledge_space_uuid",
      path: "knowledge_space_uuid",
      kind: "select",
      resource: "knowledge_space",
      required: true,
      labelKey: "workflow.fields.knowledge_space_uuid",
      placeholderKey: "workflow.editor.marketingKnowledgeSpacePlaceholder",
      hintKey: "workflow.editor.marketingKnowledgeSpaceHint",
    },
    {
      key: "source_type",
      path: "source.type",
      kind: "select",
      required: true,
      labelKey: "workflow.editor.sourceTypeLabel",
      defaultValue: "text",
      options: [
        { labelKey: "workflow.marketingInput.sourceType.text", label: "", value: "text" },
        { labelKey: "workflow.marketingInput.sourceType.audio", label: "", value: "audio" },
        { labelKey: "workflow.marketingInput.sourceType.document", label: "", value: "document" },
        { labelKey: "workflow.marketingInput.sourceType.link", label: "", value: "link" },
      ],
    },
    {
      key: "text",
      path: "source.content",
      kind: "textarea",
      rows: 7,
      required: true,
      labelKey: "workflow.editor.sourceTextLabel",
      placeholderKey: "workflow.editor.sourceTextPlaceholder",
      defaultValue: "",
      visibleWhen: { field: "source_type", equals: "text" },
    },
    {
      key: "url",
      path: "source.url",
      kind: "text",
      required: true,
      labelKey: "workflow.editor.sourceUrlLabel",
      placeholderKey: "workflow.editor.sourceUrlPlaceholder",
      defaultValue: "",
      visibleWhen: { field: "source_type", equals: "link" },
    },
    {
      key: "asset_uuid",
      path: "source.asset_uuid",
      kind: "text",
      required: true,
      labelKey: "workflow.editor.sourceAssetLabel",
      placeholderKey: "workflow.editor.sourceAssetPlaceholder",
      hintKey: "workflow.editor.sourceAssetHint",
      defaultValue: "",
      visibleWhen: { field: "source_type", equals: ["audio", "document"] },
    },
    {
      key: "language",
      path: "source.language",
      kind: "select",
      labelKey: "workflow.editor.marketingLanguageLabel",
      defaultValue: "zh",
      options: [
        { labelKey: "workflow.marketingInput.language.zh", label: "", value: "zh" },
        { labelKey: "workflow.marketingInput.language.en", label: "", value: "en" },
        { labelKey: "workflow.marketingInput.language.ja", label: "", value: "ja" },
        { labelKey: "workflow.marketingInput.language.ko", label: "", value: "ko" },
      ],
    },
    {
      key: "note",
      path: "note",
      kind: "textarea",
      rows: 3,
      labelKey: "workflow.fields.note",
      placeholderKey: "workflow.editor.marketingRunNotePlaceholder",
      defaultValue: "",
    },
  ],
};

builtinRunFormDefinitions["workflow.input.campaign_review.v1"] = {
  ...builtinRunFormDefinitions["workflow.input.knowledge_source.v1"],
  titleKey: "workflow.editor.campaignReviewRunFormTitle",
  descriptionKey: "workflow.editor.campaignReviewRunFormDescription",
};

builtinRunFormDefinitions["workflow.input.metadata_intake.v1"] = {
  titleKey: "workflow.editor.metadataIntakeRunFormTitle",
  descriptionKey: "workflow.editor.metadataIntakeRunFormDescription",
  fields: [
    {
      key: "taxonomy_namespace",
      path: "taxonomy_namespace",
      kind: "text",
      required: true,
      labelKey: "workflow.fields.taxonomy_namespace",
      defaultValue: "corex.marketing.methodology",
    },
    {
      key: "tag_namespace",
      path: "tag_namespace",
      kind: "text",
      required: true,
      labelKey: "workflow.fields.tag_namespace",
      defaultValue: "corex.marketing",
    },
    {
      key: "dictionary_namespace",
      path: "dictionary_namespace",
      kind: "text",
      required: true,
      labelKey: "workflow.fields.dictionary_namespace",
      defaultValue: "corex.marketing",
    },
    {
      key: "resource_type_namespace",
      path: "resource_type_namespace",
      kind: "text",
      required: true,
      labelKey: "workflow.fields.resource_type_namespace",
      defaultValue: "corex.knowledge",
    },
    {
      key: "intake_text",
      path: "intake.text",
      kind: "textarea",
      rows: 6,
      required: true,
      labelKey: "workflow.editor.intakeTextLabel",
      placeholderKey: "workflow.editor.intakeTextPlaceholder",
      defaultValue: "workflow_debug_metadata_intake",
    },
    {
      key: "intake_source",
      path: "intake.source",
      kind: "text",
      required: true,
      labelKey: "workflow.editor.intakeSourceLabel",
      defaultValue: "workflow_editor_run_test",
    },
  ],
};

builtinRunFormDefinitions["workflow.input.skill_review.v1"] = {
  titleKey: "workflow.editor.skillReviewRunFormTitle",
  descriptionKey: "workflow.editor.skillReviewRunFormDescription",
  fields: [
    {
      key: "skill_id",
      path: "skill_id",
      kind: "text",
      required: true,
      labelKey: "workflow.fields.skill_id",
      defaultValue: "debug.echo",
    },
    {
      key: "text",
      path: "input.text",
      kind: "textarea",
      rows: 6,
      required: true,
      labelKey: "workflow.editor.skillReviewInputTextLabel",
      placeholderKey: "workflow.editor.skillReviewInputTextPlaceholder",
      defaultValue: "workflow_debug_skill_review",
    },
  ],
};

const skillSelectItems = computed<SkillSelectOption[]>(() =>
  skillRecords.value
    .filter((skill) => skill.status === "published")
    .map((skill) => ({
      label: skillOptionLabel(skill),
      value: `${skill.skill_id}@${skill.version}`,
      skillID: skill.skill_id,
      version: skill.version,
    }))
);

const selectedNodeSkillRecord = computed(() => {
  const skillID = String(selectedNode.value?.data?.props?.skill_id || "").trim();
  const version = String(selectedNode.value?.data?.props?.skill_version || "").trim();
  if (!skillID) return null;
  return skillRecords.value.find((skill) =>
    skill.skill_id === skillID && (!version || skill.version === version)
  ) || null;
});

const selectedNodeSkill = computed<SkillSelectOption | null>({
  get() {
    const skillID = String(selectedNode.value?.data?.props?.skill_id || "").trim();
    const version = String(selectedNode.value?.data?.props?.skill_version || "").trim();
    if (!skillID) return null;
    return skillSelectItems.value.find((item) =>
      item.skillID === skillID && (!version || item.version === version)
    ) || null;
  },
  set(skillOption) {
    const node = selectedNode.value;
    if (!node || node.data.kind !== "skill.invoke") return;
    if (!skillOption) {
      node.data.props = {
        ...(node.data.props || {}),
        skill_id: "",
        skill_version: "",
        skill_source: "",
        skill_status: "",
      };
      return;
    }
    const skill = skillRecords.value.find((item) =>
      item.skill_id === skillOption.skillID && item.version === skillOption.version
    );
    node.data.props = {
      ...(node.data.props || {}),
      skill_id: skillOption.skillID,
      skill_version: skillOption.version,
      skill_source: skill?.source || "",
      skill_status: skill?.status || "",
      input_path: String(node.data.props?.input_path || "$.vars.parsed"),
      output_path: String(node.data.props?.output_path || "$.vars.extracted"),
    };
  },
});

const selectedNodeSkillDetail = computed(() => {
  const skill = selectedNodeSkillRecord.value;
  if (!skill) return "";
  return t("workflow.editor.skillSelectedHint", {
    source: skillSourceLabel(skill.source),
    version: skill.version || t("workflow.editor.notConfigured"),
    status: skillStatusLabel(skill.status),
  });
});

const skillModelModalityOptions = computed<SelectOption[]>(() => [
  { label: t("workflow.modelModality.llm"), value: "llm" },
  { label: t("workflow.modelModality.vlm"), value: "vlm" },
  { label: t("workflow.modelModality.audioAsr"), value: "audio_asr" },
  { label: t("workflow.modelModality.documentParse"), value: "document_parse" },
  { label: t("workflow.modelModality.embedding"), value: "embedding" },
]);

const selectedNodeSkillModelModality = computed<string>({
  get() {
    return String(selectedNode.value?.data?.props?.model_override?.modality || "llm");
  },
  set(modality) {
    const node = selectedNode.value;
    if (!node || node.data.kind !== "skill.invoke") return;
    node.data.props = {
      ...(node.data.props || {}),
      model_override: {
        ...(node.data.props?.model_override || {}),
        modality,
        profile_uuid: "",
        profile_label: "",
        provider: "",
        model: "",
      },
    };
  },
});

const selectedNodeSkillModelProfileItems = computed<ModelProfileSelectOption[]>(() => {
  const modality = selectedNodeSkillModelModality.value;
  return modelProfiles.value
    .filter((profile) => modelProfileMatchesModality(profile, modality))
    .map((profile) => ({
      label: modelProfileOptionLabel(profile),
      value: modelProfileOptionValue(profile),
      modality: profile.modality,
      provider: profile.provider,
      model: profile.model,
    }));
});

const selectedNodeSkillModelProfile = computed<ModelProfileSelectOption | null>({
  get() {
    const profileUUID = String(selectedNode.value?.data?.props?.model_override?.profile_uuid || "").trim();
    const provider = String(selectedNode.value?.data?.props?.model_override?.provider || "").trim();
    const model = String(selectedNode.value?.data?.props?.model_override?.model || "").trim();
    if (!profileUUID && (!provider || !model)) return null;
    return selectedNodeSkillModelProfileItems.value.find((item) =>
      item.value === profileUUID || (item.provider === provider && item.model === model)
    ) || null;
  },
  set(profileOption) {
    const node = selectedNode.value;
    if (!node || node.data.kind !== "skill.invoke") return;
    node.data.props = {
      ...(node.data.props || {}),
      model_override: profileOption
        ? {
            ...(node.data.props?.model_override || {}),
            modality: profileOption.modality,
            profile_uuid: profileOption.value,
            profile_label: profileOption.label,
            provider: profileOption.provider,
            model: profileOption.model,
          }
        : {
            ...(node.data.props?.model_override || {}),
            modality: selectedNodeSkillModelModality.value,
            profile_uuid: "",
            profile_label: "",
            provider: "",
            model: "",
          },
    };
  },
});

const metadataStrategyOptions = computed<SelectOption[]>(() => [
  { label: t("workflow.metadataStrategy.ruleBased"), value: "rule_based" },
  { label: t("workflow.metadataStrategy.llmAssisted"), value: "llm_assisted" },
  { label: t("workflow.metadataStrategy.hybrid"), value: "hybrid" },
]);

const selectedNodeMetadataStrategy = computed<string>({
  get() {
    return String(selectedNode.value?.data?.props?.classification_strategy || "rule_based");
  },
  set(strategy) {
    const node = selectedNode.value;
    if (!node || node.data.kind !== "metadata.classify") return;
    node.data.props = {
      ...(node.data.props || {}),
      classification_strategy: strategy,
    };
  },
});

const metadataTaxonomySelectItems = computed<SelectOption[]>(() =>
  metadataTaxonomies.value
    .filter((item) => item.status === "enabled")
    .map((item) => ({
      label: metadataTaxonomyOptionLabel(item),
      value: item.namespace,
    }))
);

const metadataDictionarySelectItems = computed<SelectOption[]>(() =>
  metadataDictionaries.value
    .filter((item) => item.status === "enabled")
    .map((item) => ({
      label: metadataDictionaryOptionLabel(item),
      value: item.namespace,
    }))
);

const metadataTagNamespaceSelectItems = computed<SelectOption[]>(() => {
  const namespaces = new Map<string, { label: string; count: number }>();
  for (const tag of metadataTags.value.filter((item) => item.status === "enabled")) {
    const namespace = String(tag.namespace || "").trim();
    if (!namespace) continue;
    const current = namespaces.get(namespace);
    if (current) {
      current.count += 1;
      continue;
    }
    namespaces.set(namespace, {
      label: metadataNamespaceHumanLabel(namespace),
      count: 1,
    });
  }
  return [...namespaces.entries()]
    .sort((left, right) => left[1].label.localeCompare(right[1].label))
    .map(([value, item]) => ({
      label: t("workflow.editor.metadataTagNamespaceOptionLabel", { name: item.label, count: item.count }),
      value,
    }));
});

const metadataResourceTypeSelectItems = computed<SelectOption[]>(() =>
  metadataResourceTypes.value
    .filter((item) => item.status === "enabled")
    .map((item) => ({
      label: metadataResourceTypeOptionLabel(item),
      value: item.resource_type,
    }))
);

const selectedNodeTaxonomy = computed<SelectOption | null>({
  get() {
    return selectedNodeMetadataOption("taxonomy_namespace", metadataTaxonomySelectItems.value);
  },
  set(option) {
    updateSelectedNodeMetadataProp("taxonomy_namespace", option?.value || "");
  },
});

const selectedNodeDictionary = computed<SelectOption | null>({
  get() {
    return selectedNodeMetadataOption("dictionary_namespace", metadataDictionarySelectItems.value);
  },
  set(option) {
    updateSelectedNodeMetadataProp("dictionary_namespace", option?.value || "");
  },
});

const selectedNodeTagNamespace = computed<SelectOption | null>({
  get() {
    return selectedNodeMetadataOption("tag_namespace", metadataTagNamespaceSelectItems.value);
  },
  set(option) {
    updateSelectedNodeMetadataProp("tag_namespace", option?.value || "");
  },
});

const selectedNodeResourceType = computed<SelectOption | null>({
  get() {
    return selectedNodeMetadataOption("resource_type_namespace", metadataResourceTypeSelectItems.value);
  },
  set(option) {
    updateSelectedNodeMetadataProp("resource_type_namespace", option?.value || "");
  },
});

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
        entry("skill_id", "workflow.editor.skillNodeSkillLabel", props.skill_id),
        entry("model_override", "workflow.editor.skillModelOverride", props.model_override),
        entry("input_path", "workflow.editor.businessInputSource", props.input_path),
        entry("output_path", "workflow.editor.businessOutputTarget", props.output_path),
      ];
    case "metadata.classify":
      return [
        entry("classification_strategy", "workflow.editor.metadataStrategyLabel", props.classification_strategy),
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

function formatBusinessConfigValue(key: string, value: unknown) {
  if (value === undefined || value === null || value === "") return t("workflow.editor.notConfigured");
  if (key.endsWith("_path") || key === "draft_refs_path" || key === "condition_source_path") {
    return businessPathLabel(String(value));
  }
  if (key === "skill_id") return humanizeSkillID(String(value));
  if (key === "model_override" && typeof value === "object") {
    const override = value as Record<string, any>;
    const label = String(override.profile_label || "").trim();
    const provider = String(override.provider || "").trim();
    const model = String(override.model || "").trim();
    if (label) return label;
    if (provider && model) return `${provider}/${model}`;
    return t("workflow.editor.notConfigured");
  }
  if (key === "classification_strategy") {
    const i18nKey = `workflow.metadataStrategy.${camelCase(String(value))}`;
    return te(i18nKey) ? t(i18nKey) : humanizeModuleKey(String(value));
  }
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

function editableInputRunFormFields(node: Node) {
  const form = ensureInputRunFormSchema(node);
  return form.fields;
}

function ensureInputRunFormSchema(node: Node): RunFormDefinition {
  node.data.props = node.data.props || {};
  const props = node.data.props;
  const inputSchema = props.input_schema && typeof props.input_schema === "object" && !Array.isArray(props.input_schema)
    ? props.input_schema
    : {};
  props.input_schema = inputSchema;

  const existing = inputSchema["x-run-form"] || inputSchema["x_run_form"];
  if (existing && typeof existing === "object" && Array.isArray(existing.fields)) {
    const normalizedFields = existing.fields.map((field: any) => normalizeRunFormField(field)).filter(Boolean) as RunFormField[];
    inputSchema["x-run-form"] = {
      ...existing,
      fields: normalizedFields,
    };
    delete inputSchema["x_run_form"];
    return inputSchema["x-run-form"] as RunFormDefinition;
  }

  const ref = String(props.input_schema_ref || "").trim();
  const builtin = ref ? builtinRunFormDefinitions[ref] : null;
  inputSchema["x-run-form"] = {
    title: builtin ? runFormTitleForEditor(builtin) : t("workflow.editor.startNodeRunFormTitle"),
    description: builtin ? runFormDescriptionForEditor(builtin) : t("workflow.editor.startNodeRunFormDescription"),
    fields: builtin ? builtin.fields.map(cloneRunFormFieldForEditor) : [],
  };
  return inputSchema["x-run-form"] as RunFormDefinition;
}

function cloneRunFormFieldForEditor(field: RunFormField): RunFormField {
  return {
    key: field.key,
    path: field.path || field.key,
    kind: field.kind,
    label: runFormFieldLabel(field),
    placeholder: runFormFieldPlaceholder(field),
    hint: runFormFieldHint(field),
    required: Boolean(field.required),
    rows: field.rows,
    defaultValue: field.defaultValue,
    options: runFormFieldOptions(field),
    resource: field.resource,
    visibleWhen: field.visibleWhen,
  };
}

function runFormTitleForEditor(form: RunFormDefinition) {
  if (form.titleKey && te(form.titleKey)) return t(form.titleKey);
  return form.title || t("workflow.editor.startNodeRunFormTitle");
}

function runFormDescriptionForEditor(form: RunFormDefinition) {
  if (form.descriptionKey && te(form.descriptionKey)) return t(form.descriptionKey);
  return form.description || t("workflow.editor.startNodeRunFormDescription");
}

function addInputRunFormField() {
  openInputFieldDialog(null);
}

function openInputFieldDialog(index: number | null) {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "input.capture") return;
  const form = ensureInputRunFormSchema(node);
  editingInputFieldIndex.value = index;
  const field = index === null ? null : form.fields[index] || null;
  const nextIndex = form.fields.length + 1;
  inputFieldDraft.key = field?.key || `input_${nextIndex}`;
  inputFieldDraft.path = field?.path || field?.key || `input_${nextIndex}`;
  inputFieldDraft.kind = field?.kind || "text";
  inputFieldDraft.label = field?.label || field?.key || t("workflow.editor.newInputFieldLabel", { index: nextIndex });
  inputFieldDraft.placeholder = field?.placeholder || "";
  inputFieldDraft.required = Boolean(field?.required);
  inputFieldDraft.options = inputFieldOptionsForDraft(field, inputFieldDraft.key);
  inputFieldDraft.knowledgeSpaceResource = field?.resource === "knowledge_space";
  inputFieldDialogOpen.value = true;
}

function closeInputFieldDialog() {
  inputFieldDialogOpen.value = false;
}

function saveInputFieldDialog() {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "input.capture") return;
  const form = ensureInputRunFormSchema(node);
  const key = inputFieldDraft.key.trim() || `input_${form.fields.length + 1}`;
  const path = inputFieldDraft.path.trim() || key;
  if (!inputFieldDraft.label.trim()) {
    toast.add({
      title: t("workflow.editor.inputFieldInvalidTitle"),
      description: t("workflow.editor.inputFieldInvalidDescription"),
      color: "error",
    });
    return;
  }
  const selectedOptions = inputFieldDraft.options.filter((option) => option.selected);
  if (!inputFieldDraft.knowledgeSpaceResource && inputFieldDraft.kind === "select" && selectedOptions.length === 0) {
    toast.add({
      title: t("workflow.editor.inputFieldInvalidTitle"),
      description: t("workflow.editor.inputFieldOptionsRequiredDescription"),
      color: "error",
    });
    return;
  }
  const field: RunFormField = {
    key,
    path,
    kind: inputFieldDraft.knowledgeSpaceResource ? "select" : inputFieldDraft.kind,
    label: inputFieldDraft.label.trim(),
    placeholder: inputFieldDraft.placeholder.trim(),
    required: inputFieldDraft.required,
    options: inputFieldDraft.knowledgeSpaceResource
      ? []
      : inputFieldDraft.kind === "select"
        ? selectedOptions.map(({ label, value }) => ({ label, value }))
        : [],
    resource: inputFieldDraft.knowledgeSpaceResource ? "knowledge_space" : undefined,
  };
  if (editingInputFieldIndex.value === null) {
    form.fields.push(field);
  } else {
    form.fields.splice(editingInputFieldIndex.value, 1, field);
  }
  closeInputFieldDialog();
}

function removeInputRunFormField(index: number) {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "input.capture") return;
  const form = ensureInputRunFormSchema(node);
  form.fields.splice(index, 1);
}

function handleInputFieldKindChange(value: string | number | boolean | Record<string, any> | undefined) {
  const nextKind = String(value || "") as RunFormFieldKind;
  if (nextKind === "select" && inputFieldDraft.options.length === 0) {
    inputFieldDraft.options = defaultInputFieldOptionDrafts(inputFieldDraft.key);
  }
}

function handleInputFieldKnowledgeSpaceResourceChange(value: boolean | "indeterminate") {
  const enabled = value === true;
  inputFieldDraft.knowledgeSpaceResource = enabled;
  if (enabled) {
    inputFieldDraft.kind = "select";
    return;
  }
  if (inputFieldDraft.kind === "select" && inputFieldDraft.options.length === 0) {
    inputFieldDraft.options = defaultInputFieldOptionDrafts(inputFieldDraft.key);
  }
}

function inputFieldOptionsForDraft(field: RunFormField | null, key: string): InputFieldOptionDraft[] {
  const options = field?.options || [];
  if (options.length > 0) {
    return options.map((option) => ({
      label: option.labelKey && te(option.labelKey) ? t(option.labelKey) : option.label || option.value,
      value: option.value,
      selected: true,
    }));
  }
  return field?.kind === "select" ? defaultInputFieldOptionDrafts(key) : [];
}

function defaultInputFieldOptionDrafts(key: string): InputFieldOptionDraft[] {
  const normalizedKey = key.toLowerCase();
  if (normalizedKey.includes("language") || normalizedKey.includes("locale")) {
    return [
      { label: t("workflow.marketingInput.language.zh"), value: "zh", selected: true },
      { label: t("workflow.marketingInput.language.en"), value: "en", selected: true },
      { label: t("workflow.marketingInput.language.ja"), value: "ja", selected: true },
      { label: t("workflow.marketingInput.language.ko"), value: "ko", selected: true },
    ];
  }
  return [
    { label: t("workflow.marketingInput.sourceType.text"), value: "text", selected: true },
    { label: t("workflow.marketingInput.sourceType.audio"), value: "audio", selected: true },
    { label: t("workflow.marketingInput.sourceType.document"), value: "document", selected: true },
    { label: t("workflow.marketingInput.sourceType.link"), value: "link", selected: true },
  ];
}

function inputFieldDisplayLabel(field: RunFormField, index: number) {
  return field.label || field.key || t("workflow.editor.newInputFieldLabel", { index: index + 1 });
}

function inputFieldKindLabel(kind: RunFormFieldKind) {
  const key = `workflow.inputFieldKind.${kind}`;
  return te(key) ? t(key) : kind;
}

function normalizeSelectedNodeProps(node: Node) {
  node.data.props = node.data.props || {};
  if (node.data.kind === "human.review") {
    node.data.props.approver_policy = node.data.props.approver_policy || { roles: [] };
  }
  if (node.data.kind === "input.capture") {
    node.data.props.source_policy = node.data.props.source_policy || { text: false, form: false };
    node.data.props.artifact_output_path = String(node.data.props.artifact_output_path || "$.artifacts.source");
    ensureInputRunFormSchema(node);
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
  if (node.data.kind === "skill.invoke") {
    void loadSkillNodeReferenceData({ notify: false });
  }
  if (node.data.kind === "metadata.classify") {
    void loadMetadataReferenceData({ notify: false });
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
  const tasks: Promise<void>[] = [];
  if (runFormFields.value.some((field) => field.resource === "capability")) {
    tasks.push(loadCapabilityReferenceData({ notify: true }));
  }
  if (runFormFields.value.some((field) => field.resource === "knowledge_space")) {
    tasks.push(loadKnowledgeSpaceReferenceData({ notify: true }));
  }
  await Promise.all(tasks);
  syncRunFormSelects();
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
	    syncRunFormSelects();
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

async function loadKnowledgeSpaceReferenceData(options: { notify?: boolean } = {}) {
  if (knowledgeSpaces.value.length || knowledgeSpacesLoading.value) return;
  const notify = options.notify ?? true;
  knowledgeSpacesLoading.value = true;
  knowledgeSpacesError.value = "";
  try {
    knowledgeSpaces.value = await knowledgeSpacesApi.listSpaces({ limit: 100, status: "active" });
    syncRunFormSelects();
    if (!marketingKnowledgeSpaceSelectItems.value.length) {
      knowledgeSpacesError.value = t("workflow.editor.marketingKnowledgeSpacesEmpty");
      if (notify) {
        toast.add({
          title: t("workflow.editor.marketingKnowledgeSpacesLoadFailed"),
          description: knowledgeSpacesError.value,
          color: "error",
        });
      }
    }
  } catch (err: any) {
    knowledgeSpacesError.value = err?.message || t("workflow.editor.marketingKnowledgeSpacesLoadFailed");
    if (notify) {
      toast.add({
        title: t("workflow.editor.marketingKnowledgeSpacesLoadFailed"),
        description: knowledgeSpacesError.value,
        color: "error",
      });
    }
  } finally {
    knowledgeSpacesLoading.value = false;
  }
}

async function loadSkillNodeReferenceData(options: { notify?: boolean } = {}) {
  await Promise.all([
    loadSkillReferenceData(options),
    loadModelProfileReferenceData(options),
  ]);
}

async function loadSkillReferenceData(options: { notify?: boolean } = {}) {
  if (skillRecords.value.length || skillsLoading.value) return;
  const notify = options.notify ?? true;
  skillsLoading.value = true;
  skillsError.value = "";
  try {
    const resp = await skillsService.list({ status: "published", page: 1, page_size: 200 });
    skillRecords.value = resp?.data?.items || [];
    if (!skillSelectItems.value.length) {
      skillsError.value = t("workflow.editor.skillOptionsEmpty");
      if (notify) {
        toast.add({
          title: t("workflow.editor.skillOptionsLoadFailed"),
          description: skillsError.value,
          color: "error",
        });
      }
    }
  } catch (err: any) {
    skillsError.value = err?.message || t("workflow.editor.skillOptionsLoadFailed");
    if (notify) {
      toast.add({
        title: t("workflow.editor.skillOptionsLoadFailed"),
        description: skillsError.value,
        color: "error",
      });
    }
  } finally {
    skillsLoading.value = false;
  }
}

async function loadModelProfileReferenceData(options: { notify?: boolean } = {}) {
  if (modelProfiles.value.length || modelProfilesLoading.value) return;
  const notify = options.notify ?? true;
  modelProfilesLoading.value = true;
  modelProfilesError.value = "";
  try {
    const result = await AISettingService.getProfiles("default", ["llm", "vlm", "audio_asr", "embedding"]);
    modelProfiles.value = result.profiles || [];
    if (!modelProfiles.value.length) {
      modelProfilesError.value = t("workflow.editor.modelProfileOptionsEmpty");
      if (notify) {
        toast.add({
          title: t("workflow.editor.modelProfileOptionsLoadFailed"),
          description: modelProfilesError.value,
          color: "error",
        });
      }
    }
  } catch (err: any) {
    modelProfilesError.value = err?.message || t("workflow.editor.modelProfileOptionsLoadFailed");
    if (notify) {
      toast.add({
        title: t("workflow.editor.modelProfileOptionsLoadFailed"),
        description: modelProfilesError.value,
        color: "error",
      });
    }
  } finally {
    modelProfilesLoading.value = false;
  }
}

async function loadMetadataReferenceData(options: { notify?: boolean } = {}) {
  if (
    (metadataTaxonomies.value.length || metadataDictionaries.value.length || metadataTags.value.length || metadataResourceTypes.value.length) ||
    metadataOptionsLoading.value
  ) {
    return;
  }
  const notify = options.notify ?? true;
  metadataOptionsLoading.value = true;
  metadataOptionsError.value = "";
  try {
    const [taxonomies, dictionaries, tags, resourceTypes] = await Promise.all([
      metadataGovernanceService.listTaxonomies({ page: 1, page_size: 200, status: "enabled" }),
      metadataGovernanceService.listDictionaries({ page: 1, page_size: 200, status: "enabled" }),
      metadataGovernanceService.listTags({ page: 1, page_size: 500, status: "enabled" }),
      metadataGovernanceService.listResourceTypes({ page: 1, page_size: 200, status: "enabled" }),
    ]);
    metadataTaxonomies.value = taxonomies.items || [];
    metadataDictionaries.value = dictionaries.items || [];
    metadataTags.value = tags.items || [];
    metadataResourceTypes.value = resourceTypes.items || [];
    if (
      !metadataTaxonomySelectItems.value.length ||
      !metadataDictionarySelectItems.value.length ||
      !metadataTagNamespaceSelectItems.value.length ||
      !metadataResourceTypeSelectItems.value.length
    ) {
      metadataOptionsError.value = t("workflow.editor.metadataOptionsIncomplete");
      if (notify) {
        toast.add({
          title: t("workflow.editor.metadataOptionsLoadFailed"),
          description: metadataOptionsError.value,
          color: "error",
        });
      }
    }
  } catch (err: any) {
    metadataOptionsError.value = err?.message || t("workflow.editor.metadataOptionsLoadFailed");
    if (notify) {
      toast.add({
        title: t("workflow.editor.metadataOptionsLoadFailed"),
        description: metadataOptionsError.value,
        color: "error",
      });
    }
  } finally {
    metadataOptionsLoading.value = false;
  }
}

function knowledgeSpaceOptionLabel(space: KnowledgeSpaceRecord) {
  const name = String(space.spaceName || "").trim();
  const department = String(space.departmentCode || "").trim();
  if (name && department) return t("workflow.editor.marketingKnowledgeSpaceOptionLabel", { name, department });
  return name || t("workflow.editor.marketingKnowledgeSpaceUnnamed");
}

function skillOptionLabel(skill: SkillRecord) {
  const name = humanizeSkillID(skill.skill_id);
  return t("workflow.editor.skillOptionLabel", {
    name,
    source: skillSourceLabel(skill.source),
    version: skill.version || t("workflow.editor.notConfigured"),
  });
}

function skillSourceLabel(source: string) {
  const key = `workflow.skillSource.${camelCase(source)}`;
  return te(key) ? t(key) : humanizeModuleKey(source);
}

function skillStatusLabel(status: string) {
  const key = `workflow.skillStatus.${camelCase(status)}`;
  return te(key) ? t(key) : humanizeModuleKey(status);
}

function modelProfileMatchesModality(profile: AgentProfile, modality: string) {
  const profileModality = String(profile.modality || "").trim();
  if (modality === "document_parse") return profileModality === "llm" || profileModality === "vlm";
  return profileModality === modality;
}

function modelProfileOptionValue(profile: AgentProfile) {
  return String(profile.uuid || `${profile.provider}/${profile.model}`).trim();
}

function modelProfileOptionLabel(profile: AgentProfile) {
  const label = String(profile.label || "").trim();
  const provider = String(profile.provider || "").trim();
  const model = String(profile.model || "").trim();
  if (label && provider && model) {
    return t("workflow.editor.modelProfileOptionLabel", { label, provider, model });
  }
  if (provider && model) return `${provider}/${model}`;
  return t("workflow.editor.modelProfileUnnamed");
}

function selectedNodeMetadataOption(key: string, items: SelectOption[]) {
  const value = String(selectedNode.value?.data?.props?.[key] || "").trim();
  if (!value) return null;
  return items.find((item) => item.value === value) || null;
}

function updateSelectedNodeMetadataProp(key: string, value: string) {
  const node = selectedNode.value;
  if (!node || node.data.kind !== "metadata.classify") return;
  node.data.props = {
    ...(node.data.props || {}),
    [key]: value,
  };
}

function metadataTaxonomyOptionLabel(item: Taxonomy) {
  return t("workflow.editor.metadataGovernanceOptionLabel", {
    name: item.display_name || metadataNamespaceHumanLabel(item.namespace),
    namespace: item.namespace,
  });
}

function metadataDictionaryOptionLabel(item: DictionaryNamespace) {
  return t("workflow.editor.metadataGovernanceOptionLabel", {
    name: item.display_name || metadataNamespaceHumanLabel(item.namespace),
    namespace: item.namespace,
  });
}

function metadataResourceTypeOptionLabel(item: ResourceType) {
  return t("workflow.editor.metadataGovernanceOptionLabel", {
    name: item.display_name || metadataNamespaceHumanLabel(item.resource_type),
    namespace: item.resource_type,
  });
}

function metadataNamespaceHumanLabel(namespace: string) {
  return humanizeModuleKey(String(namespace || "").replace(/^corex\./, ""));
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
    if (runFormConfigError.value) {
      throw new Error(runFormConfigError.value);
    }
    if (runFormFields.value.length > 0) {
      return buildDebugInputFromRunForm();
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
  if (runFormFields.value.length > 0) {
    applyRunFormInput(input);
    syncRunFormSelects();
  }
  debugInputText.value = JSON.stringify(input, null, 2);
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
  if (runFormFields.value.length > 0) {
    return buildDebugInputFromRunForm(false);
  }
  return {};
}

function runFormDefinitionFromSchema(schema?: Record<string, any>): RunFormDefinition | null {
  if (!schema || typeof schema !== "object") return null;
  const explicit = (schema["x-run-form"] || schema["x_run_form"]) as Record<string, any> | undefined;
  if (explicit && Array.isArray(explicit.fields)) {
    return {
      title: typeof explicit.title === "string" ? explicit.title : undefined,
      titleKey: typeof explicit.titleKey === "string" ? explicit.titleKey : typeof explicit.title_i18n_key === "string" ? explicit.title_i18n_key : undefined,
      description: typeof explicit.description === "string" ? explicit.description : undefined,
      descriptionKey:
        typeof explicit.descriptionKey === "string"
          ? explicit.descriptionKey
          : typeof explicit.description_i18n_key === "string"
            ? explicit.description_i18n_key
            : undefined,
      fields: explicit.fields.map(normalizeRunFormField).filter(Boolean) as RunFormField[],
    };
  }
  const properties = schema.properties;
  if (!properties || typeof properties !== "object") return null;
  const required = new Set(Array.isArray(schema.required) ? schema.required.map(String) : []);
  const fields = Object.entries(properties).map(([key, property]) => {
    const spec = (property || {}) as Record<string, any>;
    return normalizeRunFormField({
      key,
      path: key,
      kind: schemaFieldKind(spec),
      label: spec.title || key,
      placeholder: spec.description || "",
      required: required.has(key),
      defaultValue: spec.default,
      options: Array.isArray(spec.enum)
        ? spec.enum.map((value: any) => ({ label: String(value), value: String(value) }))
        : undefined,
    });
  }).filter(Boolean) as RunFormField[];
  return fields.length > 0 ? { fields } : null;
}

function normalizeRunFormField(raw: Record<string, any>): RunFormField | null {
  const key = String(raw.key || raw.name || "").trim();
  if (!key) return null;
  const resource = raw.resource === "knowledge_space" || raw.resource === "capability" ? raw.resource : undefined;
  const kind = normalizeRunFormFieldKind(String(raw.kind || raw.type || (resource ? "select" : "text")));
  const options = Array.isArray(raw.options)
    ? raw.options.map((item: any) => ({
      label: String(item.label || item.value || ""),
      labelKey: typeof item.labelKey === "string" ? item.labelKey : typeof item.label_i18n_key === "string" ? item.label_i18n_key : undefined,
      value: String(item.value ?? ""),
    })).filter((item: SelectOption) => item.value)
    : undefined;
  return {
    key,
    path: String(raw.path || key).trim(),
    kind,
    label: typeof raw.label === "string" ? raw.label : undefined,
    labelKey: typeof raw.labelKey === "string" ? raw.labelKey : typeof raw.label_i18n_key === "string" ? raw.label_i18n_key : undefined,
    placeholder: typeof raw.placeholder === "string" ? raw.placeholder : undefined,
    placeholderKey:
      typeof raw.placeholderKey === "string" ? raw.placeholderKey : typeof raw.placeholder_i18n_key === "string" ? raw.placeholder_i18n_key : undefined,
    hint: typeof raw.hint === "string" ? raw.hint : undefined,
    hintKey: typeof raw.hintKey === "string" ? raw.hintKey : typeof raw.hint_i18n_key === "string" ? raw.hint_i18n_key : undefined,
    required: Boolean(raw.required),
    rows: Number(raw.rows || 0) || undefined,
    defaultValue: raw.defaultValue ?? raw.default,
    options,
    resource,
    visibleWhen: raw.visibleWhen || raw.visible_when,
  };
}

function schemaFieldKind(spec: Record<string, any>): RunFormFieldKind {
  if (Array.isArray(spec.enum)) return "select";
  if (spec.format === "textarea" || spec["x-ui"] === "textarea") return "textarea";
  if (spec.type === "boolean") return "boolean";
  if (spec.type === "number" || spec.type === "integer") return "number";
  if (spec.type === "object" || spec.type === "array") return "object";
  return "text";
}

function normalizeRunFormFieldKind(kind: string): RunFormFieldKind {
  if (kind === "textarea" || kind === "select" || kind === "boolean" || kind === "number" || kind === "object") return kind;
  if (kind === "integer") return "number";
  return "text";
}

function isRunFormFieldVisible(field: RunFormField) {
  const condition = field.visibleWhen;
  if (!condition?.field) return true;
  const actual = String(runFormValues[condition.field] ?? "");
  if (typeof condition.equals !== "undefined") {
    const expected = Array.isArray(condition.equals) ? condition.equals.map(String) : [String(condition.equals)];
    return expected.includes(actual);
  }
  if (typeof condition.notEquals !== "undefined") {
    const denied = Array.isArray(condition.notEquals) ? condition.notEquals.map(String) : [String(condition.notEquals)];
    return !denied.includes(actual);
  }
  return true;
}

function runFormFieldLabel(field: RunFormField) {
  if (field.labelKey && te(field.labelKey)) return t(field.labelKey);
  return field.label || humanizeModuleKey(field.key);
}

function runFormFieldPlaceholder(field: RunFormField) {
  if (field.placeholderKey && te(field.placeholderKey)) return t(field.placeholderKey);
  return field.placeholder || "";
}

function runFormFieldHint(field: RunFormField) {
  if (field.hintKey && te(field.hintKey)) return t(field.hintKey);
  return field.hint || "";
}

function runFormFieldOptions(field: RunFormField) {
  return (field.options || []).map((option) => ({
    label: option.labelKey && te(option.labelKey) ? t(option.labelKey) : option.label,
    value: option.value,
  }));
}

function genericSelectValue(field: RunFormField) {
  return runFormSelectValues.value[field.key] || null;
}

function setGenericSelectValue(field: RunFormField, option: SelectOption | null) {
  runFormSelectValues.value = {
    ...runFormSelectValues.value,
    [field.key]: option,
  };
  runFormValues[field.key] = option?.value || "";
}

function applyRunFormInput(input: Record<string, any>) {
  for (const key of Object.keys(runFormValues)) delete runFormValues[key];
  runFormSelectValues.value = {};
  for (const field of runFormFields.value) {
    const value = valueAtPath(input, field.path);
    runFormValues[field.key] = typeof value === "undefined" ? defaultRunFormFieldValue(field) : formValueForField(field, value);
  }
}

function syncRunFormSelects() {
  const next: Record<string, SelectOption | null> = {};
  for (const field of runFormFields.value) {
    const value = String(runFormValues[field.key] ?? "");
    if (field.resource === "knowledge_space") {
      next[field.key] = marketingKnowledgeSpaceSelectItems.value.find((item) => item.value === value) || null;
    } else if (field.resource === "capability") {
      next[field.key] = genericCapabilitySelectItems.value.find((item) => item.value === value) || null;
    }
  }
  runFormSelectValues.value = next;
}

function defaultRunFormFieldValue(field: RunFormField) {
  if (typeof field.defaultValue !== "undefined") return field.defaultValue;
  if (field.kind === "boolean") return false;
  if (field.kind === "number") return 0;
  if (field.kind === "object") return "{}";
  return "";
}

function formValueForField(field: RunFormField, value: any) {
  if (field.kind === "object") return typeof value === "string" ? value : JSON.stringify(value ?? {}, null, 2);
  if (field.kind === "boolean") return Boolean(value);
  if (field.kind === "number") return Number(value);
  return typeof value === "undefined" || value === null ? "" : String(value);
}

function buildDebugInputFromRunForm(validate = true) {
  const result: Record<string, any> = {};
  for (const field of visibleRunFormFields.value) {
    if (field.resource === "knowledge_space" && validate && knowledgeSpacesLoading.value) {
      throw new Error(t("workflow.editor.marketingKnowledgeSpacesLoading"));
    }
    if (field.resource === "capability" && validate && capabilityOptionsLoading.value) {
      throw new Error(t("workflow.editor.capabilityOptionsLoading"));
    }
    const rawValue = runFormValues[field.key];
    if (validate && field.required && isEmptyRunFormValue(rawValue)) {
      throw new Error(t("workflow.editor.runFormFieldRequired", { field: runFormFieldLabel(field) }));
    }
    assignPath(result, field.path, payloadValueForField(field, rawValue));
  }
  return result;
}

function isEmptyRunFormValue(value: any) {
  if (typeof value === "boolean") return false;
  if (typeof value === "number") return !Number.isFinite(value);
  return String(value ?? "").trim() === "";
}

function payloadValueForField(field: RunFormField, value: any) {
  if (field.kind === "boolean") return Boolean(value);
  if (field.kind === "number") return Number(value);
  if (field.kind === "object") {
    if (typeof value !== "string") return value ?? {};
    const trimmed = value.trim();
    if (!trimmed) return {};
    return JSON.parse(trimmed);
  }
  return typeof value === "string" ? value.trim() : value;
}

function valueAtPath(source: Record<string, any>, path: string) {
  return path.split(".").filter(Boolean).reduce((current: any, segment) => current?.[segment], source);
}

function assignPath(target: Record<string, any>, path: string, value: any) {
  const parts = path.split(".").filter(Boolean);
  if (parts.length === 0) return;
  let cursor = target;
  for (let index = 0; index < parts.length - 1; index += 1) {
    const part = parts[index];
    if (!cursor[part] || typeof cursor[part] !== "object" || Array.isArray(cursor[part])) {
      cursor[part] = {};
    }
    cursor = cursor[part];
  }
  cursor[parts[parts.length - 1]] = value;
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
  padding: 12px 12px 14px;
  scroll-padding-top: 12px;
}

.properties-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
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
  gap: 10px;
  margin-top: 10px;
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

.input-fields-editor {
  display: grid;
  gap: 8px;
  border-top: 1px solid var(--wf-border);
  padding-top: 10px;
}

.input-fields-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 0;
}

.input-fields-header div {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.input-fields-header strong {
  color: var(--wf-text);
  font-size: 13px;
}

.input-fields-header span {
  color: var(--wf-muted);
  font-size: 12px;
  line-height: 1.45;
}

.input-field-list {
  display: grid;
  gap: 6px;
}

.input-field-list-item {
  display: grid;
  gap: 6px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel);
  padding: 8px 8px 7px;
}

.input-field-list-head {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.input-field-list-head strong {
  min-width: 0;
  overflow: hidden;
  color: var(--wf-text);
  font-size: 13px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.input-field-list-badges,
.input-field-list-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.input-field-list-actions {
  flex: 0 0 auto;
  justify-content: flex-end;
}

.input-field-badge {
  padding: 0 5px;
  font-size: 10px;
  line-height: 1.3;
}

.input-field-empty {
  border: 1px dashed var(--wf-border);
  border-radius: 8px;
  padding: 14px;
  color: var(--wf-muted);
  font-size: 12px;
  text-align: center;
}

.input-field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.input-field-dialog-form {
  display: grid;
  width: 100%;
  gap: 12px;
  padding: 0;
}

:deep(.input-field-dialog [data-slot="body"]) {
  width: 100%;
}

:deep(.input-field-dialog [data-slot="footer"]) {
  width: 100%;
}

.input-field-dialog-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  width: 100%;
}

.input-field-editor-section {
  display: grid;
  gap: 10px;
  width: 100%;
}

.input-field-editor-section + .input-field-editor-section {
  border-top: 1px solid color-mix(in srgb, var(--wf-border) 74%, transparent);
  padding-top: 10px;
}

.input-field-editor-section-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-field-editor-section-title::after {
  display: block;
  flex: 1;
  height: 1px;
  background: color-mix(in srgb, var(--wf-border) 70%, transparent);
  content: "";
}

.input-field-editor-section-title strong {
  color: var(--wf-dim);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0;
  text-transform: uppercase;
}

.input-field-option-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.input-field-option-choice {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel);
  padding: 9px 10px;
  color: var(--wf-text);
  font-size: 13px;
}

.input-field-flags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.input-field-flags label {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--wf-border);
  border-radius: 8px;
  background: var(--wf-panel);
  padding: 8px 10px;
  color: var(--wf-text);
  font-size: 12px;
}

@media (max-width: 760px) {
  .input-field-grid,
  .input-field-option-grid {
    grid-template-columns: 1fr;
  }
}

.skill-model-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

@media (max-width: 1280px) {
  .skill-model-grid {
    grid-template-columns: 1fr;
  }
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

.run-start-node-card {
  grid-column: 1 / -1;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  border: 1px solid color-mix(in srgb, var(--wf-accent) 34%, var(--wf-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-accent) 7%, var(--wf-panel));
  padding: 12px;
}

.run-start-node-icon {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border-radius: 8px;
  background: color-mix(in srgb, var(--wf-accent) 18%, var(--wf-panel-soft));
  color: var(--wf-accent);
}

.run-start-node-content {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.run-start-node-content span {
  color: var(--wf-muted);
  font-size: 12px;
}

.run-start-node-content strong {
  overflow: hidden;
  color: var(--wf-text);
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.run-start-node-meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
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
