<template>
  <section class="min-w-0 space-y-4" data-testid="archive-governance">
 <ArchiveCommandConfirmationDialog v-if="overview?.command_confirmation_version === 1" v-model:open="commandConfirmationOpen" :record-id="commandRecordId" :revision="propsRevision" :connection-status="events.status.value" @attention="commandConfirmationAttention=$event" @busy="commandConfirmationBusy=$event" @changed="reload"/>
 <ArchiveInterruptionDialog v-if="overview?.interruption_resolution_version === 1" v-model:open="interruptionOpen" :job-id="interruptionJob" :closure-available="overview?.interruption_request_closure_version === 1" :revision="propsRevision" :connection-status="events.status.value" @attention="dispositionAttention=$event" @busy="dispositionBusy=$event" @changed="reload"/>
    <div v-if="!configurationOnly" class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold">{{ t(props.mediaMode?'backupCenter.onDemand.mediaSchedules':'storageGov.archive.title') }}</h2>
      <div class="flex flex-wrap gap-2"><UButton color="neutral" variant="outline" @click="gcRunsOpen = true">{{ t('storageGov.archive.gcRuns') }}</UButton><UButton color="neutral" variant="outline" :loading="loading" @click="reload">{{ t('storageGov.refresh') }}</UButton></div>
    </div>
    <UAlert v-if="!configurationOnly" color="info" :title="t('storageGov.archive.optIn')" :description="t(props.mediaMode?'backupCenter.onDemand.advancedHint':'storageGov.archive.scopeHint')" />
    <UAlert v-if="dispositionAttention" color="warning" :title="t('storageGov.archive.disposition.unknown')" :description="t('storageGov.archive.disposition.unknownHint')" data-testid="archive-disposition-attention"><template #actions><UButton color="neutral" variant="outline" @click="interruptionOpen=true">{{ t('storageGov.archive.disposition.title') }}</UButton></template></UAlert>
    <UAlert v-if="overview?.command_execution?.unconfirmed" color="warning" variant="soft" :ui="{title:'text-highlighted',description:'text-toned'}" :title="t('storageGov.archive.commands.unconfirmed')" :description="t('storageGov.archive.commands.protection', {count:overview.command_execution.unconfirmed})" data-testid="archive-command-unconfirmed"><template #actions><div class="flex flex-wrap gap-2"><UButton v-for="id in overview.command_execution.jobs || []" :key="id" color="neutral" variant="outline" class="break-all" @click="openInterruption(id)">{{ id }}</UButton><UButton v-for="id in overview.command_execution.jobs || []" v-if="overview.command_confirmation_version===1" :key="'command-'+id" color="warning" variant="outline" data-testid="archive-command-review-open" @click="commandRecordId=id;commandConfirmationOpen=true">{{ t('storageGov.archive.commandConfirmation.title') }}</UButton><UButton v-for="record in (overview.probe_command_confirmation_version===1 ? overview.command_execution.records||[] : []).filter(r=>r.subject==='probe')" :key="'probe-'+record.record_id" color="warning" variant="outline" class="max-w-full break-words" data-testid="archive-probe-command-review-open" @click="commandRecordId=record.record_id;commandConfirmationOpen=true">{{ t('storageGov.archive.commandConfirmation.probe.open', {name:record.name||record.record_id}) }}</UButton></div></template></UAlert>
    <p v-else-if="overview?.command_execution?.active" class="text-sm text-muted" role="status" data-testid="archive-command-active">{{ t('storageGov.archive.commands.active', {count:overview.command_execution.active}) }}</p>
    <UAlert v-if="overview?.remote_execution?.enabled === false" color="warning" variant="soft" :title="t('storageGov.release.remoteUnavailable')" :description="t('storageGov.release.remoteHint')" data-testid="archive-release-unavailable" />
    <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
    <UAlert v-if="pendingRun" color="warning" :title="t('storageGov.archive.runRequest.pending')" :description="t('storageGov.archive.runRequest.hint')" data-testid="archive-run-pending">
      <template #actions><div class="min-w-0 space-y-2"><p class="break-all text-xs" data-testid="archive-run-request-id">{{ pendingRun.request_id }}</p><p class="break-all text-xs">{{ pendingRun.policy_id }} · {{ t('storageGov.archive.runRequest.revision', { revision: pendingRun.policy_revision }) }}</p><UButton color="neutral" variant="outline" :disabled="busy" data-testid="archive-run-reconcile" @click="reconcileRun">{{ t('storageGov.archive.runRequest.reconcile') }}</UButton></div></template>
    </UAlert>
    <UAlert v-if="overview && (overview.run_request_version !== 1 || overview.execution_version !== 1)" color="warning" :title="t('storageGov.archive.runRequest.upgradeRequired')" data-testid="archive-run-upgrade" />
    <UAlert v-if="interruptedJobs.length" color="warning" :title="t('storageGov.archive.execution.attention')" :description="t('storageGov.archive.execution.hint')" data-testid="archive-execution-attention"><template #actions><div class="flex min-w-0 flex-wrap gap-2"><UButton v-for="job in interruptedJobs.slice(0, 3)" :key="job.id" color="neutral" variant="outline" class="max-w-full" :disabled="busy" @click="openInterruption(job.id)"><span class="truncate">{{ job.id }}</span></UButton></div></template></UAlert>
    <UAlert v-if="runContextProblem" color="warning" :title="t('storageGov.archive.runRequest.contextUnavailable')" :description="runContextProblem" data-testid="archive-run-context-error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <div v-if="resolvedRun" class="flex min-w-0 flex-wrap items-center gap-2 rounded border border-default p-3 text-sm" data-testid="archive-run-recorded"><span class="break-all">{{ t('storageGov.archive.runRequest.recorded') }} · {{ resolvedRun.job_id }}</span><UButton v-if="!configurationOnly && !mediaMode" color="neutral" variant="outline" :disabled="busy" data-testid="archive-run-view-job" @click="showRunJob">{{ t('storageGov.archive.runRequest.viewJob') }}</UButton></div>
    <UAlert v-if="events.status.value === 'disconnected'" color="warning" :title="t('storageGov.archive.disconnected')" />
    <p v-if="overview?.custody" class="text-sm text-muted">{{ t('storageGov.archive.executor') }} · {{ overview.custody.node_id }}</p>
    <UAlert v-if="overview?.custody?.legacy_detected && !overview.custody.managed" color="warning" :title="t('storageGov.archive.handoffBlocked')" />
    <UAlert v-if="overview?.scheduler_started === false" color="warning" :title="t('storageGov.archive.workerOff')" />
    <div v-if="overview && !configurationOnly" class="flex flex-wrap gap-4 rounded border border-default p-3 text-sm"><span>{{ t('storageGov.archive.activeCount') }} {{ policies.filter(p => p.enabled && p.execution_ready).length }}</span><span>{{ t('storageGov.archive.pendingCount') }} {{ overview.log_pending_transfers ?? '—' }}</span><span>{{ t('storageGov.archive.lastSuccess') }} {{ time(lastSuccess) }}</span></div>
    <UAlert v-if="pendingConfiguration" color="warning" :title="t('storageGov.resourceConfig.unconfirmed')"><template #actions><UButton :disabled="busy" data-testid="archive-config-reconcile" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template></UAlert>
    <UAlert v-if="pendingTarget" color="warning" :title="t('storageGov.resourceConfig.unconfirmed')"><template #actions><UButton :disabled="busy" data-testid="target-reconcile" @click="reconcileTarget">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template></UAlert>
    <ArchiveTargetControlStatus v-if="overview && (configurationOnly || section === 'targets')" :configuration="targetControl" :disabled="busy || !!pendingTarget" @adopt="targetAdoptOpen=true" />
    <ArchivePolicyControlStatus v-if="overview && (configurationOnly || section === 'policies')" :configuration="policyControl" :disabled="busy || !!pendingConfiguration || !!pendingTarget" @adopt="policyAdoptOpen=true" />
    <UAlert v-if="overview && !configurationOnly && section !== 'policies' && (policyControl?.authority !== 'database' || policyControl?.problem)" color="warning" :title="t('storageGov.policyControl.attention')"><template #actions><UButton color="neutral" variant="outline" @click="section='policies'">{{ t('storageGov.archive.policies') }}</UButton></template></UAlert>
    <div v-if="configurationOnly" class="space-y-4" data-testid="resource-archive-settings">
      <div class="flex flex-wrap items-center justify-between gap-2"><h4 class="font-semibold">{{ t('storageGov.resourceConfig.current') }}</h4><UButton color="neutral" variant="ghost" :loading="loading" :disabled="!!pendingConfiguration || busy" @click="reload">{{ t('storageGov.refresh') }}</UButton></div>
      <p v-if="loading && !overview" role="status">{{ t('storageGov.loading') }}</p>
      <template v-if="overview">
        <label v-if="policies.length" class="block space-y-1 text-sm"><span>{{ t('storageGov.archive.policies') }}</span><select v-model="selectedPolicyId" :disabled="!!pendingConfiguration || busy" class="min-h-11 w-full rounded-lg border border-default bg-default px-3" data-testid="resource-archive-policy-select"><option v-for="p in policies" :key="p.id" :value="p.id">{{ p.name }}</option></select></label>
        <p v-else class="text-sm text-muted">{{ t('storageGov.archive.noPolicies') }}</p>
        <dl v-if="currentPolicy" class="grid gap-3 text-sm sm:grid-cols-2">
          <div><dt class="text-muted">{{ t('storageGov.resourceConfig.status') }}</dt><dd class="mt-1">{{ t(currentPolicy.enabled && !currentPolicy.execution_ready ? 'storageGov.policyControl.configured' : currentPolicy.enabled ? 'storageGov.archive.enabled' : 'storageGov.archive.disabled') }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.archive.source') }}</dt><dd class="mt-1">{{ sourceName(currentPolicy.source_id) }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.archive.remoteDays') }}</dt><dd class="mt-1">{{ currentPolicy.remote_retention_days }} {{ t('storageGov.runtime.dayUnit') }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.resourceConfig.sourceLimit') }}</dt><dd class="mt-1">{{ bytes(currentPolicy.remote_max_bytes) }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.resourceConfig.remoteCleanup') }}</dt><dd class="mt-1">{{ t(currentPolicy.remote_cleanup_enabled ? 'storageGov.archive.enabled' : 'storageGov.archive.disabled') }}</dd></div>
        </dl>
        <div class="flex flex-wrap gap-2">
          <UButton :disabled="!policyCanWrite || busy || !sources.length" data-testid="resource-archive-policy-edit" @click="editPolicy(currentPolicy)">{{ t(currentPolicy ? 'storageGov.resourceConfig.edit' : 'storageGov.archive.newPolicy') }}</UButton>
          <UButton v-if="currentPolicy" color="neutral" variant="outline" :disabled="!policyCanWrite || busy" @click="preparePolicyStatus(currentPolicy,'archive')">{{ t(currentPolicy.enabled ? 'storageGov.archive.disable' : 'storageGov.archive.enable') }}</UButton>
          <UButton v-if="currentPolicy" color="neutral" variant="outline" :disabled="!policyCanWrite || busy" @click="preparePolicyStatus(currentPolicy,'gc')">{{ t(currentPolicy.remote_cleanup_enabled ? 'storageGov.archive.gcDisable' : 'storageGov.archive.gcEnable') }}</UButton>
          <UButton v-if="currentPolicy && (currentPolicy.enabled || currentPolicy.remote_cleanup_enabled)" color="neutral" variant="outline" :disabled="busy || !policyCanWrite" data-testid="resource-policy-pause" @click="preparePolicyStatus(currentPolicy,'pause')">{{ t('storageGov.policyControl.pause') }}</UButton>
          <UButton v-if="currentPolicy" color="neutral" variant="ghost" :disabled="!policyCanWrite || busy" @click="editPolicy()">{{ t('storageGov.archive.newPolicy') }}</UButton>
        </div>
        <div class="space-y-3 border-t border-default pt-4">
          <div class="flex flex-wrap items-center justify-between gap-2"><h5 class="font-medium">{{ t('storageGov.archive.target') }}</h5><span v-if="currentTarget" class="text-xs" :class="currentTarget.verification_valid ? 'text-success' : 'text-warning'">{{ t(currentTarget.verification_valid ? 'storageGov.archive.verified' : 'storageGov.archive.unverified') }}</span></div>
          <p v-if="!currentTarget" class="text-sm text-muted">{{ t('storageGov.archive.noTargets') }}</p>
          <dl v-else class="grid gap-3 text-sm sm:grid-cols-2"><div class="min-w-0 sm:col-span-2"><dt class="text-muted">{{ t('storageGov.archive.endpoint') }}</dt><dd class="mt-1 break-all">{{ currentTarget.endpoint }}</dd></div><div><dt class="text-muted">{{ t('storageGov.archive.bucket') }}</dt><dd class="mt-1 break-all">{{ currentTarget.bucket }}</dd></div><div><dt class="text-muted">{{ t('storageGov.archive.prefix') }}</dt><dd class="mt-1 break-all">{{ currentTarget.prefix }}</dd></div></dl>
          <div class="flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="!targetCanWrite || busy" data-testid="resource-archive-target-edit" @click="editTarget(currentTarget)">{{ t(currentTarget ? 'storageGov.resourceConfig.editTarget' : 'storageGov.archive.newTarget') }}</UButton><UButton v-if="currentTarget" color="neutral" variant="outline" :disabled="busy || !targetCanWrite || targetControl?.desired_revision !== targetControl?.applied_revision" @click="perform(() => api.testTarget(currentTarget!.id!, operationController?.signal))">{{ t('storageGov.archive.test') }}</UButton><UButton color="neutral" variant="ghost" :disabled="!!pendingConfiguration || busy" @click="targetPickerOpen = true">{{ t('storageGov.resourceConfig.allTargets') }}</UButton><UButton color="neutral" variant="ghost" :disabled="!!pendingConfiguration || busy" @click="credentialOpen = true">{{ t('storageGov.credentialVault.title') }}</UButton></div>
        </div>
      </template>
    </div>
    <UModal v-model:open="targetPickerOpen" :title="t('storageGov.archive.targets')" :description="t('storageGov.archive.targetHint')"><template #body><div class="space-y-3"><p v-if="!targets.length">{{ t('storageGov.archive.noTargets') }}</p><article v-for="target in targets" :key="target.id" class="rounded-lg border border-default p-3"><p class="font-medium">{{ target.name }}</p><p class="mt-1 break-all text-sm text-muted">{{ target.endpoint }} / {{ target.bucket }}</p><UButton class="mt-2" color="neutral" variant="outline" :disabled="!targetCanWrite || busy" @click="targetPickerOpen = false; editTarget(target)">{{ t('storageGov.resourceConfig.edit') }}</UButton></article><UButton :disabled="!targetCanWrite || busy" @click="targetPickerOpen = false; editTarget()">{{ t('storageGov.archive.newTarget') }}</UButton></div></template></UModal>
    <UModal :open="!!retryCandidate" @update:open="v => { if (!v && !busy) retryCandidate=undefined; }" :dismissible="!busy" :title="t('storageGov.archive.execution.retry')" :description="t('storageGov.archive.execution.retryHint')"><template #body><p class="break-all text-sm">{{ retryCandidate?.id }}</p></template><template #footer><UButton color="neutral" variant="outline" :disabled="busy" @click="retryCandidate=undefined">{{ t('common.cancel') }}</UButton><UButton :loading="busy" data-testid="archive-retry-confirm" @click="retryOriginal">{{ t('storageGov.archive.execution.confirm') }}</UButton></template></UModal>
    <BackupSectionTabs v-if="!configurationOnly" v-model="section" :items="sections" :label="t(props.mediaMode?'backupCenter.onDemand.mediaSchedules':'storageGov.archive.title')">
      <template #catalog>
        <form class="mb-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-[minmax(12rem,1fr)_10rem_10rem_auto]" @submit.prevent="page = 1; loadCatalog()">
          <UInput :disabled="busy" v-model="query" :placeholder="t('storageGov.archive.searchHint')" :aria-label="t('storageGov.archive.searchHint')" class="min-w-0 w-full sm:col-span-2 xl:col-span-1" data-testid="archive-search" />
          <UInput :disabled="busy" v-model="from" type="date" :aria-label="t('storageGov.archive.from')" class="min-w-0 w-full" />
          <UInput :disabled="busy" v-model="to" type="date" :aria-label="t('storageGov.archive.to')" class="min-w-0 w-full" />
          <UButton type="submit" :loading="catalogLoading" class="justify-self-start">{{ t('storageGov.archive.search') }}</UButton>
        </form>
        <p v-if="!catalogLoading && !catalog?.items.length" class="py-6 text-muted">{{ t('storageGov.archive.empty') }}</p>
        <div class="space-y-3">
          <article v-for="job in catalog?.items" :key="job.id" :data-archive-job-id="job.id" class="rounded-lg border border-default p-4" data-testid="archive-catalog-item">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0"><p class="break-words font-medium">{{ job.source_file_name || sourceName(job.source_id) }}</p><p class="text-sm text-muted">{{ time(job.created_at) }} · {{ bytes(job.size_bytes) }} · {{ job.file_count }} {{ t('storageGov.archive.files') }}</p></div>
              <UBadge :color="job.remote_state === 'success' ? 'success' : 'warning'" variant="subtle" data-testid="archive-remote-state">{{ t('storageGov.archive.remoteStates.' + remoteState(job)) }}</UBadge>
            </div>
            <p class="mt-2 break-all text-xs text-muted" data-testid="archive-job-id">{{ t('storageGov.archive.jobId') }} · {{ job.id }}</p>
            <p v-if="executionState(job) !== 'recorded'" class="mt-2 text-sm" data-testid="archive-execution-state">{{ t('storageGov.archive.execution.states.' + executionState(job)) }}</p>
            <p class="mt-2 text-sm" data-testid="archive-local-state">{{ t('storageGov.archive.localStates.' + localState(job)) }}</p>
            <p v-if="job.status === 'success' && ['pending', 'failed', 'unconfirmed'].includes(job.remote_state)" class="mt-2 text-sm text-warning" data-testid="archive-transfer-warning">{{ t('storageGov.archive.transferUnconfirmed') }}</p>
            <p v-if="(job.imported || job.recovered || job.migrated_from) && !job.provenance_verified" class="mt-2 text-sm text-warning">{{ t('storageGov.archive.originUnconfirmed') }}</p>
            <ArchiveCaptureRetentionDetails v-if="job.capture_retention" :value="job.capture_retention"/>
            <p v-if="job.status !== 'retained' && (job.error || job.remote_error)" class="mt-2 text-sm text-error">{{ job.error || job.remote_error }}</p>
            <div class="mt-3 flex flex-wrap gap-2">
              <UButton v-if="job.status === 'interrupted' && overview?.interruption_resolution_version === 1" color="warning" variant="outline" :disabled="busy" data-testid="archive-disposition-open" @click="openInterruption(job.id)">{{ t('storageGov.archive.disposition.title') }}</UButton>
              <UButton v-if="job.interruption_pending && job.status === 'success' && job.target && !job.imported && !job.recovered" color="warning" variant="outline" :disabled="busy || executionState(job) === 'active' || overview?.execution_version !== 1" data-testid="archive-retry-original" @click="retryCandidate=job">{{ t('storageGov.archive.execution.retry') }}</UButton>
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="inspect(job)">{{ t('storageGov.archive.preview') }}</UButton>
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="perform(() => api.download(job.id, operationController?.signal))">{{ t('storageGov.archive.download') }}</UButton>
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="prepareMigration(job)">{{ t('storageGov.archive.migrate') }}</UButton>
              <UButton v-if="job.status !== 'retained'" color="neutral" variant="ghost" :disabled="busy || job.remote_state === 'expired' || job.imported || job.recovered" @click="perform(() => api.protect(job.id, !job.protected, operationController?.signal))">{{ t(job.imported || job.recovered ? 'storageGov.archive.legacyProtected' : job.protected ? 'storageGov.archive.protected' : 'storageGov.archive.protect') }}</UButton>
            </div>
          </article>
        </div>
        <div v-if="catalog" class="mt-4 flex flex-wrap items-center gap-3 text-sm">
          <UButton color="neutral" variant="outline" :disabled="page <= 1 || catalogLoading" @click="page--; loadCatalog()">{{ t('storageGov.archive.previous') }}</UButton>
          <span>{{ page }} / {{ Math.max(1, catalog.pagination.pages) }} · {{ catalog.pagination.total }}</span>
          <UButton color="neutral" variant="outline" :disabled="page >= catalog.pagination.pages || catalogLoading" @click="page++; loadCatalog()">{{ t('storageGov.archive.next') }}</UButton>
        </div>
      </template>
      <template #policies>
        <div class="mb-4 flex justify-end"><UButton :disabled="busy || !sources.length || !policyCanWrite" data-testid="policy-new" @click="editPolicy()">{{ t('storageGov.archive.newPolicy') }}</UButton></div>
        <p v-if="!policies.length" class="py-6 text-muted">{{ t('storageGov.archive.noPolicies') }}</p>
        <article v-for="policy in policies" :key="policy.id" :data-policy-id="policy.id" class="mb-3 rounded-lg border border-default p-4">
          <div class="flex flex-wrap items-center justify-between gap-3"><p class="font-medium">{{ policy.name }}</p><UBadge :color="policy.enabled ? 'success' : 'neutral'">{{ t(policy.enabled && !policy.execution_ready ? 'storageGov.policyControl.configured' : policy.enabled ? 'storageGov.archive.enabled' : 'storageGov.archive.disabled') }}</UBadge></div>
          <p class="mt-2 text-sm">{{ sourceName(policy.source_id) }} → {{ targetName(policy.target_id) }}</p><p v-if="(policy.enabled || policy.remote_cleanup_enabled) && !policy.execution_ready" class="mt-2 text-sm text-warning" role="status">{{ t('storageGov.policyControl.notReady') }}</p><UButton v-if="policy.enabled || policy.remote_cleanup_enabled" class="mt-2" color="neutral" variant="outline" :disabled="busy || !policyCanWrite" data-testid="policy-pause" @click="preparePolicyStatus(policy,'pause')">{{ t('storageGov.policyControl.pause') }}</UButton>
          <p class="mt-1 text-sm text-muted">{{ policy.remote_retention_days }} {{ t('storageGov.runtime.dayUnit') }} · {{ bytes(policy.remote_max_bytes) }}</p>
          <div class="mt-3 flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="busy || !policyCanWrite" data-testid="policy-edit" @click="editPolicy(policy)">{{ t('storageGov.runtime.configure') }}</UButton><UButton :disabled="busy || !policyCanWrite" :color="policy.enabled ? 'warning' : 'primary'" data-testid="policy-toggle" @click="preparePolicyStatus(policy,'archive')">{{ t(policy.enabled ? 'storageGov.archive.disable' : 'storageGov.archive.enable') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy || !policy.enabled || !policy.execution_ready || !!pendingConfiguration || overview?.run_request_version !== 1 || overview?.execution_version !== 1 || policyBlocked(policy) || !runContextReady || !!pendingRun || !!runContextProblem" data-testid="archive-run-start" @click="runPolicy(policy)">{{ t('storageGov.archive.run') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy || !policyCanWrite" data-testid="policy-gc-toggle" @click="preparePolicyStatus(policy,'gc')">{{ t(policy.remote_cleanup_enabled ? 'storageGov.archive.gcDisable' : 'storageGov.archive.gcEnable') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="previewGC(policy)">{{ t('storageGov.archive.gcPreview') }}</UButton></div>
        </article>
      </template>
      <template #targets>
        <div class="mb-4 flex flex-wrap gap-2"><UButton :disabled="busy || !targetCanWrite" data-testid="target-new" @click="editTarget()">{{ t('storageGov.archive.newTarget') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="credentialOpen = true">{{ t('storageGov.credentialVault.title') }}</UButton></div>
        <p v-if="!targets.length" class="py-6 text-muted">{{ t('storageGov.archive.noTargets') }}</p>
        <article v-for="target in targets" :key="target.id" :data-target-id="target.id" class="mb-3 rounded-lg border border-default p-4">
          <div class="flex flex-wrap justify-between gap-2"><h3 class="font-medium">{{ target.name }}</h3><UBadge :color="target.verification_valid ? 'success' : 'warning'">{{ t(target.verification_valid ? 'storageGov.archive.verified' : 'storageGov.archive.unverified') }}</UBadge></div>
          <p class="mt-2 break-all text-sm">{{ target.endpoint }} / {{ target.bucket }} / {{ target.prefix }}</p>
          <p class="mt-1 text-sm text-muted">{{ t('storageGov.archive.executor') }}: {{ target.executor || overview?.instance }} · {{ time(target.verified_at) }}</p>
          <div class="mt-3 flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="busy || !targetCanWrite" data-testid="target-edit" @click="editTarget(target)">{{ t('storageGov.targetControl.edit') }}</UButton><UButton :disabled="busy || !targetCanWrite || targetControl?.desired_revision !== targetControl?.applied_revision" color="neutral" data-testid="target-test" @click="perform(() => api.testTarget(target.id!, operationController?.signal))">{{ t('storageGov.archive.test') }}</UButton><UButton :disabled="busy || !target.verification_valid" color="neutral" variant="outline" @click="importCandidate = target">{{ t('storageGov.archive.import') }}</UButton></div>
        </article>
      </template>
    </BackupSectionTabs>

    <UModal :open="!!gcPolicyCandidate" :title="t('storageGov.archive.gcSchedule')" :description="t('storageGov.archive.gcScheduleHint')" @update:open="open => { if (!open && !busy) gcPolicyCandidate = undefined }">
      <template #body><p>{{ gcPolicyCandidate?.name }} · {{ gcPolicyCandidate?.remote_retention_days }} {{ t('storageGov.runtime.dayUnit') }}</p><p class="mt-2 text-sm text-muted">{{ t('storageGov.policyControl.confirmVersion',{revision:statusBaseRevision,target:statusTargetRevision}) }}</p><UButton v-if="pendingConfiguration" :disabled="busy" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton><UButton v-if="policyConflict" :disabled="busy || !!pendingConfiguration" @click="reloadPolicyStatus">{{ t('storageGov.policyControl.refreshConfirmation') }}</UButton><p v-if="error" role="alert" class="text-error">{{ error }}</p></template><template #footer><UButton :loading="busy" :disabled="!!pendingConfiguration || !policyCanWrite" data-testid="policy-status-confirm" @click="toggleGC">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!gcPreview" :title="t('storageGov.archive.gcPreview')" :description="t('storageGov.archive.gcHint')" @update:open="open => { if (!open && !busy) gcPreview = undefined }">
      <template #body><p>{{ gcPreview?.candidate_versions }} {{ t('storageGov.archive.versions') }} · {{ bytes(gcPreview?.candidate_bytes) }}</p><p v-if="error" role="alert" class="text-error">{{ error }}</p><details v-if="gcPreview?.protected"><summary>{{ t('storageGov.archive.protectionDetails') }}</summary><p v-for="(count, reason) in gcPreview.protected" :key="reason">{{ reason }} · {{ count }}</p></details></template><template #footer><UButton color="error" :loading="busy" :disabled="!gcPreview?.candidate_versions" @click="startGC">{{ t('storageGov.archive.gcConfirm') }}</UButton></template>
    </UModal>
    <UModal v-model:open="gcRunsOpen" :title="t('storageGov.archive.gcRuns')" :description="t('storageGov.archive.gcHint')">
      <template #body><p v-if="!overview?.remote_runs?.length" class="text-muted">{{ t('storageGov.archive.noRuns') }}</p><article v-for="run in overview?.remote_runs" :key="run.id" class="mb-3 rounded border border-default p-3"><p>{{ time(run.created_at) }} · {{ t('storageGov.archive.gcStates.' + run.state) }}</p><p>{{ ['unconfirmed', 'running', 'queued'].includes(run.state) ? '—' : bytes(run.confirmed_bytes) }}</p><p v-if="run.problem" class="text-warning">{{ run.problem }}</p><UButton v-if="['unconfirmed', 'running', 'queued'].includes(run.state)" color="neutral" variant="outline" :disabled="busy" @click="perform(() => api.reconcileRemoteGC(run.id, operationController?.signal))">{{ t('storageGov.archive.reconcile') }}</UButton></article></template>
    </UModal>
    <UModal v-model:open="policyAdoptOpen" :title="t('storageGov.policyControl.adopt')" :description="t('storageGov.policyControl.adoptHint')" :dismissible="!busy" :close="!busy"><template #body><p>{{ t('storageGov.policyControl.legacyCount',{count:policyControl?.legacy_count||0}) }}</p><p class="mt-3 break-all font-mono text-xs text-muted">{{ policyControl?.legacy_digest }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p><UButton v-if="pendingConfiguration" class="mt-3" :disabled="busy" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template><template #footer><UButton :disabled="!!pendingConfiguration || !policyControl?.ready" :loading="busy" data-testid="policy-adopt-confirm" @click="adoptPolicies">{{ t('backupCenter.confirm') }}</UButton></template></UModal>
    <UModal v-model:open="targetAdoptOpen" :title="t('storageGov.targetControl.adopt')" :description="t('storageGov.targetControl.adoptHint')" :dismissible="!busy" :close="!busy">
      <template #body><p>{{ t('storageGov.targetControl.legacyCount', { count: targetControl?.legacy_count || 0 }) }}</p><p class="mt-3 break-all font-mono text-xs text-muted">{{ targetControl?.legacy_digest }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p><UButton v-if="pendingTarget" class="mt-3" :disabled="busy" @click="reconcileTarget">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template>
      <template #footer><UButton :loading="busy" :disabled="!!pendingTarget || !targetControl?.ready" data-testid="target-adopt-confirm" @click="adoptTargets">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal v-model:open="targetOpen" :title="t('storageGov.archive.target')" :description="t('storageGov.archive.targetHint')" :dismissible="!busy" :close="!busy">
      <template #body><p class="mb-3 text-sm text-muted">{{ t('storageGov.targetControl.editHint', { revision: targetBaseRevision }) }}</p><UForm id="s3-archive-target" :state="targetForm" :schema="targetSchema" class="space-y-4" @submit="saveTarget">
        <UFormField name="name" :label="t('storageGov.archive.name')"><UInput :disabled="busy || !!pendingTarget" v-model="targetForm.name" data-testid="target-field-name" class="w-full" /></UFormField>
        <UFormField name="endpoint" :label="t('storageGov.archive.endpoint')"><UInput :disabled="busy || !!pendingTarget" v-model="targetForm.endpoint" data-testid="target-field-endpoint" placeholder="https://storage.example.com" class="w-full" /></UFormField>
        <UFormField name="bucket" :label="t('storageGov.archive.bucket')"><UInput :disabled="busy || !!pendingTarget" v-model="targetForm.bucket" data-testid="target-field-bucket" class="w-full" /></UFormField>
        <UFormField name="prefix" :label="t('storageGov.archive.prefix')"><UInput :disabled="busy || !!pendingTarget" v-model="targetForm.prefix" data-testid="target-field-prefix" class="w-full" /></UFormField>
        <UFormField name="region" :label="t('storageGov.archive.region')"><UInput :disabled="busy || !!pendingTarget" v-model="targetForm.region" data-testid="target-field-region" class="w-full" /></UFormField>
        <UFormField name="credential_ref" :label="t('storageGov.archive.credential')"><USelect :disabled="busy || !!pendingTarget" v-model="targetForm.credential_ref" data-testid="target-field-credential_ref" :items="overview?.s3_credentials || []" class="w-full" /></UFormField>
        <UCheckbox :disabled="busy || !!pendingTarget" v-model="targetForm.force_path_style" data-testid="target-field-force_path_style" :label="t('storageGov.archive.pathStyle')" />
        <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
      </UForm><UButton v-if="targetConflict" class="mt-3" :disabled="busy || !!pendingTarget" data-testid="target-load-latest" @click="loadLatestTarget">{{ t('storageGov.targetControl.loadLatest') }}</UButton><UButton v-if="pendingTarget" class="mt-3" :disabled="busy" @click="reconcileTarget">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton><UButton v-if="pendingConfiguration" :disabled="busy" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template>
      <template #footer><UButton type="submit" form="s3-archive-target" :loading="busy" :disabled="!!pendingConfiguration || !targetCanWrite">{{ t('storageGov.archive.saveDraft') }}</UButton></template>
    </UModal>
    <UModal v-model:open="credentialOpen" :title="t('storageGov.credentialVault.title')" :description="t('storageGov.credentialVault.description')" :dismissible="!credentialBusy" :close="credentialBusy ? false : undefined" :ui="{ content:'max-w-4xl' }">
      <template #body><ArchiveCredentialManager :revision="propsRevision" @busy="credentialBusy=$event" @changed="refreshCredentials" /></template>
    </UModal>
    <UModal v-model:open="policyOpen" :title="t('storageGov.archive.newPolicy')" :description="t('storageGov.archive.policyHint')" :dismissible="!busy" :close="!busy">
      <template #body><p class="mb-3 text-sm text-muted">{{ t('storageGov.policyControl.editHint',{revision:policyBaseRevision}) }}</p><UForm id="log-archive-policy" :state="policyForm" :schema="policySchema" class="space-y-4" @submit="savePolicy">
        <UFormField name="name" :label="t('storageGov.archive.name')"><UInput :disabled="busy || !!pendingConfiguration" v-model="policyForm.name" data-testid="policy-field-name" class="w-full" /></UFormField>
        <UFormField name="source_id" :label="t('storageGov.archive.source')"><USelect :disabled="busy || !!pendingConfiguration" v-model="policyForm.source_id" data-testid="policy-field-source_id" :items="sources.map(s => ({ value: s.id, label: s.name }))" class="w-full" /></UFormField>
        <UFormField name="target_id" :label="t('storageGov.archive.target')"><USelect :disabled="busy || !!pendingConfiguration" v-model="policyTargetChoice" data-testid="policy-field-target_id" :items="policyTargetOptions" class="w-full" /></UFormField>
        <div class="grid gap-3 sm:grid-cols-2"><UFormField name="remote_retention_days" :label="t('storageGov.archive.remoteDays')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyForm.remote_retention_days" data-testid="policy-field-remote_retention_days" type="number" class="w-full" /></UFormField><UFormField name="budgetGiB" :label="t('storageGov.archive.budget')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyBudget" data-testid="policy-field-budget" type="number" min="0" max="1048576" step="0.01" class="w-full" /></UFormField></div>
        <UFormField name="interval_minutes" :label="t('storageGov.archive.interval')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyForm.interval_minutes" data-testid="policy-field-interval_minutes" type="number" class="w-full" /></UFormField>
        <p class="text-sm text-muted">{{ t('storageGov.policyControl.budgetHint') }}</p>
        <UFormField name="remote_retention_count" :label="t('storageGov.policyControl.remoteCount')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyForm.remote_retention_count" data-testid="policy-field-remote_retention_count" type="number" min="1" max="10000" class="w-full" /></UFormField>
        <UCheckbox v-if="policyIsMedia" :disabled="busy || !!pendingConfiguration" v-model="policyForm.include_database" :label="t('backupCenter.archives.includeDatabase')" />
        <details :open="!policyForm.target_id" class="rounded-lg border border-default p-3"><summary class="cursor-pointer text-sm font-medium">{{ t('storageGov.policyControl.localCopies') }}</summary><div class="mt-3 grid gap-3 sm:grid-cols-2"><UFormField name="retention_days" :label="t('storageGov.policyControl.localDays')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyForm.retention_days" data-testid="policy-field-retention_days" type="number" min="1" max="3650" class="w-full" /></UFormField><UFormField name="retention_count" :label="t('storageGov.policyControl.localCount')"><UInput :disabled="busy || !!pendingConfiguration" v-model.number="policyForm.retention_count" data-testid="policy-field-retention_count" type="number" min="1" max="10000" class="w-full" /></UFormField></div></details>
        <p class="text-sm text-muted">{{ t('storageGov.archive.cleanupSeparate') }}</p><p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
      </UForm><UButton v-if="policyConflict" class="mt-3" :disabled="busy || !!pendingConfiguration" data-testid="policy-load-latest" @click="loadLatestPolicy">{{ t('storageGov.policyControl.loadLatest') }}</UButton><UButton v-if="pendingConfiguration" :disabled="busy" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton></template><template #footer><UButton type="submit" form="log-archive-policy" :loading="busy" :disabled="!!pendingConfiguration || !policyCanWrite">{{ t('storageGov.archive.saveDraft') }}</UButton></template>
    </UModal>
    <UModal :open="!!enableCandidate" :title="t(statusAction==='pause'?'storageGov.policyControl.pause':enableCandidate?.enabled ? 'storageGov.archive.disable' : 'storageGov.archive.enable')" :description="t('storageGov.archive.enableHint')" @update:open="open => { if (!open && !busy) enableCandidate = undefined }">
      <template #body><p>{{ enableCandidate?.name }} → {{ targetName(enableCandidate?.target_id || '') }}</p><p class="mt-2 text-sm text-muted">{{ t('storageGov.policyControl.confirmVersion',{revision:statusBaseRevision,target:statusTargetRevision}) }}</p><UButton v-if="pendingConfiguration" :disabled="busy" @click="reconcileConfiguration">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton><UButton v-if="policyConflict" :disabled="busy || !!pendingConfiguration" @click="reloadPolicyStatus">{{ t('storageGov.policyControl.refreshConfirmation') }}</UButton><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template>
      <template #footer><UButton :loading="busy" :disabled="!!pendingConfiguration || !policyCanWrite" data-testid="policy-status-confirm" @click="togglePolicy">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!importCandidate" :title="t('storageGov.archive.import')" :description="t('storageGov.archive.importHint')" @update:open="open => { if (!open && !busy) importCandidate = undefined }">
      <template #body><p class="break-all">{{ importCandidate?.bucket }}/{{ importCandidate?.prefix }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template>
      <template #footer><UButton :loading="busy" @click="importLegacy">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!migrationJob" :title="t('storageGov.archive.migrate')" :description="t('storageGov.archive.migrationHint')" @update:open="open => { if (!open && !busy) migrationJob = undefined }"><template #body><UFormField :label="t('storageGov.archive.migrationPolicy')"><USelect :disabled="busy" v-model="migrationPolicy" :items="policies.filter(p => p.enabled && sources.some(s => s.id === p.source_id && s.kind === migrationJob?.kind)).map(p => ({ label: p.name + ' → ' + targetName(p.target_id), value: p.id! }))" class="w-full" /></UFormField><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template><template #footer><UButton :disabled="!migrationPolicy" :loading="busy" @click="migrate">{{ t('backupCenter.confirm') }}</UButton></template></UModal>
    <UModal v-model:open="previewOpen" :title="t('storageGov.archive.preview')" :description="t('storageGov.archive.previewHint')" :ui="{ content: 'max-w-4xl' }">
      <template #body><div class="space-y-3"><USelect :disabled="busy" v-model="previewEntry" :items="previewJob?.files?.map(f => ({ value: f.path, label: f.path })) || []" class="w-full" @update:model-value="previewText = ''; readPreview()" /><UButton color="neutral" :loading="busy" @click="readPreview">{{ t('storageGov.archive.readPreview') }}</UButton><p v-if="error" role="alert" class="text-error">{{ error }}</p><p v-if="previewTruncated" class="text-warning">{{ t('storageGov.archive.truncated') }}</p><pre class="max-h-[55vh] overflow-auto whitespace-pre-wrap break-words rounded border border-default p-3 text-xs" data-testid="archive-log-preview">{{ previewText }}</pre></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import ArchiveCaptureRetentionDetails from './ArchiveCaptureRetentionDetails.vue';
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import BackupSectionTabs from './BackupSectionTabs.vue';
import ArchiveInterruptionDialog from './ArchiveInterruptionDialog.vue';
import ArchiveCommandConfirmationDialog from './ArchiveCommandConfirmationDialog.vue';
import ArchiveCredentialManager from './ArchiveCredentialManager.vue';
import ArchiveTargetControlStatus from './ArchiveTargetControlStatus.vue';
import ArchivePolicyControlStatus from './ArchivePolicyControlStatus.vue';
import { useArchiveOpsService, type ArchiveCatalog, type ArchiveJob, type ArchiveOverview, type ArchivePolicy, type ArchiveTarget, type ArchiveTargetChange, type ArchiveTargetMutation, type ArchivePolicyMutation, type ArchivePolicyChange, type RemoteGCResult } from '~/composables/api/services/archiveOpsService';
import { useBackupEvents } from '~/composables/useBackupEvents';
import { archiveExecutionState as executionState, archiveExecutionNeedsReview as executionNeedsReview } from '~/utils/archive-execution';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
import { AUTH_SESSION_EVENT, readAuthSession } from '~/utils/auth-session';
import { ARCHIVE_RUN_REQUEST_EVENT, archiveRunOwnerKey, beginArchiveRunIntent, readArchiveRunIntent, settleArchiveRunIntent, type ArchiveRunIntent, type ArchiveRunOwner, type ArchiveRunReceipt } from '~/utils/archive-run-request';
const props = defineProps<{ configurationOnly?: boolean; mediaMode?: boolean }>();
const emit = defineEmits<{ busy: [value: boolean]; changed: []; attention: [value: boolean] }>();
const credentialBusy=ref(false),propsRevision=ref(0);
const commandConfirmationOpen=ref(false),commandRecordId=ref(''),commandConfirmationAttention=ref(false),commandConfirmationBusy=ref(false);
function refreshCredentials(){void loadOverview();}
const selectedPolicyId = ref(''), targetPickerOpen = ref(false);
const interruptionOpen=ref(false),interruptionJob=ref(''),dispositionAttention=ref(false),dispositionBusy=ref(false);
function openInterruption(id:string){if(overview.value?.interruption_resolution_version===1){interruptionJob.value=id;interruptionOpen.value=true}else{query.value=id;page.value=1;section.value='catalog';void loadCatalog()}}
const pendingConfiguration = ref<{key:string;revision:number;policyId?:string;draftKey?:string;action:'save'|'status'|'adopt'}>();
const policyAdoptOpen=ref(false),policyConflict=ref(false),policyBaseRevision=ref(0),statusBaseRevision=ref(0),statusTargetRevision=ref(0),statusTargetFingerprint=ref(''),statusAction=ref<'archive'|'gc'|'pause'>('archive');
const pendingTarget = ref<{ key: string; action: 'save'|'adopt'; draftKey?: string; revision:number; targetId?:string }>();
const targetAdoptOpen=ref(false),targetConflict=ref(false),targetBaseRevision=ref(0);
const pendingRun = ref<ArchiveRunIntent>(), resolvedRun = ref<ArchiveRunReceipt>(), runContextReady = ref(false), runContextProblem = ref('');

const { t } = useI18n(), api = useArchiveOpsService(), GiB = 1024 ** 3;
const section = ref(props.mediaMode?'policies':'catalog'), loading = ref(false), catalogLoading = ref(false), busy = ref(false), error = ref(''), notice = ref('');
const migrationJob = ref<ArchiveJob>(), migrationPolicy = ref(''); let migrationKey = '';
const gcRunsOpen = ref(false), gcPreview = ref<RemoteGCResult>(), gcPolicyCandidate = ref<ArchivePolicy>();
const overview = ref<ArchiveOverview>(), catalog = ref<ArchiveCatalog>();
const query = ref(''), from = ref(''), to = ref(''), page = ref(1);
const interruptedJobs = computed(() => [...new Map([...[...(overview.value?.jobs || []), ...(catalog.value?.items || [])].filter(executionNeedsReview), ...(overview.value?.execution_attention || [])].map(j => [j.id, j])).values()]);
watch([pendingConfiguration,pendingTarget,pendingRun,runContextProblem,interruptedJobs,dispositionAttention,()=>overview.value?.command_execution?.unconfirmed,commandConfirmationAttention], ([a,b,c,d,jobs,pendingDisposition,commands,commandAttention]) => emit('attention', !!a||!!b||!!c||!!d||jobs.length>0||pendingDisposition||!!commands||commandAttention));
const policyBlocked = (policy: ArchivePolicy) => interruptedJobs.value.some(j => j.policy_id === policy.id);
const retryCandidate = ref<ArchiveJob>();
async function retryOriginal() { const job=retryCandidate.value, generation=operationGeneration; if (!job) return; await perform(async () => { await api.retry(job.id, operationController?.signal); if (!disposed && generation === operationGeneration) retryCandidate.value=undefined; }); }

const targetOpen = ref(false), credentialOpen = ref(false), policyOpen = ref(false), previewOpen = ref(false);
const enableCandidate = ref<ArchivePolicy>(), importCandidate = ref<ArchiveTarget>();
const previewJob = ref<ArchiveJob>(), previewEntry = ref(''), previewText = ref(''), previewTruncated = ref(false);
let operationController: AbortController | undefined;
let runOwner: ArchiveRunOwner | undefined, operationGeneration = 0;
let disposed = false, overviewController: AbortController | undefined, catalogController: AbortController | undefined;
const sections = computed(() => (props.mediaMode?['policies','targets']:['catalog', 'policies', 'targets']).map(value => ({ value, label: t('storageGov.archive.' + value) })));
const logKinds = new Set(['postgresql_logs', 'system_logs', 'audit_logs', 'application_logs', 'journal']);
const sources = computed(() => overview.value?.sources.filter(s => props.mediaMode?s.kind==='media':logKinds.has(s.kind)) || []);
const policies = computed(() => overview.value?.policies.filter(p => sources.value.some(s => s.id === p.source_id)) || []);
const targets = computed(() => overview.value?.targets.filter(s => s.driver === 's3') || []);
const currentPolicy = computed(() => policies.value.find(p => p.id === selectedPolicyId.value));
const policyControl=computed(()=>overview.value?.policy_configuration);
const policyCanWrite=computed(()=>policyControl.value?.authority==='database'&&policyControl.value.ready&&!pendingConfiguration.value&&!pendingTarget.value);
const targetControl = computed(() => overview.value?.target_configuration);
const targetCanWrite = computed(() => targetControl.value?.authority === 'database' && targetControl.value.ready && !pendingTarget.value && !pendingConfiguration.value);
const currentTarget = computed(() => currentPolicy.value ? targets.value.find(t => t.id === currentPolicy.value!.target_id) : targets.value[0]);
watch(policies, items => { if (!items.some(p => p.id === selectedPolicyId.value)) selectedPolicyId.value = items[0]?.id || ''; });
watch(()=>busy.value||credentialBusy.value||dispositionBusy.value||commandConfirmationBusy.value, value => emit('busy', value));
const lastSuccess = computed(() => overview.value?.jobs.filter(j => logKinds.has(j.kind) && j.remote_state === 'success').sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0]?.created_at);
const sourceName = (id: string) => overview.value?.sources.find(s => s.id === id)?.name || id;
const targetName = (id: string) => overview.value?.targets.find(s => s.id === id)?.name || '—';
const time = (value?: string) => value ? new Date(value).toLocaleString() : '—';
const remoteState = (job: ArchiveJob) => ['success', 'pending', 'failed', 'unconfirmed', 'expired', 'expiry_pending', 'not_requested'].includes(job.remote_state) ? job.remote_state : 'unknown';
const localState = (job: ArchiveJob) => job.status === 'running' ? (executionState(job) === 'active' ? 'running' : 'unknown') : ['success', 'failed', 'pruned', 'interrupted', 'retained'].includes(job.status) ? job.status : 'unknown';
const readable = (job: ArchiveJob) => job.status === 'success' && !['expired', 'expiry_pending'].includes(job.remote_state) || job.status === 'pruned' && job.remote_state === 'success';
const targetDefaults = (): ArchiveTarget => ({ name: '', driver: 's3', revision: 0, host: '', port: 0, user: '', directory: '', credential_ref: '', endpoint: '', bucket: '', prefix: '', region: 'us-east-1', force_path_style: true });
const targetForm = reactive(targetDefaults());

const policyDefaults = (): ArchivePolicy => ({ name: '', revision: 0, remote_cleanup_enabled: false, source_id: '', target_id: '', enabled: false, include_database: false, interval_minutes: 5, retention_days: 7, retention_count: 168, remote_retention_days: 180, remote_retention_count: 10000, remote_max_bytes: GiB });
const policyForm = reactive(policyDefaults()), policyBudget = ref(1);
const policyIsMedia=computed(()=>sources.value.find(s=>s.id===policyForm.source_id)?.kind==='media');
const policyTargetChoice=computed({get:()=>policyForm.target_id||(policyIsMedia.value?'local-only':undefined),set:(value:string|undefined)=>{policyForm.target_id=value==='local-only'?'':value||''}});
const policyTargetOptions=computed(()=>[...(policyIsMedia.value?[{value:'local-only',label:t('backupCenter.archives.localOnly')}]:[]),...targets.value.map(t=>({value:t.id!,label:t.name}))]);

const required = v.pipe(v.string(), v.minLength(1));
const targetSchema = v.object({ name: required, endpoint: v.pipe(required, v.url()), bucket: required, prefix: required, region: v.string(), credential_ref: required, force_path_style: v.boolean() });

const boundedInteger=(max:number)=>v.pipe(v.number(),v.integer(),v.minValue(1),v.maxValue(max));
const policySchema=v.object({name:required,source_id:required,target_id:v.string(),interval_minutes:boundedInteger(525600),remote_retention_days:boundedInteger(3650),remote_retention_count:boundedInteger(10000),retention_days:boundedInteger(3650),retention_count:boundedInteger(10000)});
async function loadOverview() { overviewController?.abort(); const c = overviewController = new AbortController(); loading.value = true; try { const value = await api.overview(c.signal); if (!disposed && !c.signal.aborted) overview.value = value; } catch (e) { if (!disposed && !c.signal.aborted) error.value = message(e); } finally { if (!disposed && !c.signal.aborted) loading.value = false; } }
async function loadCatalog() { catalogController?.abort(); const c = catalogController = new AbortController(); catalogLoading.value = true; try { const value = await api.catalog({ q: query.value, page: page.value, page_size: 20, from: from.value ? new Date(from.value + 'T00:00:00').toISOString() : undefined, to: to.value ? new Date(to.value + 'T23:59:59').toISOString() : undefined }, c.signal); if (!disposed && !c.signal.aborted) catalog.value = value; } catch (e) { if (!disposed && !c.signal.aborted) error.value = message(e); } finally { if (!disposed && !c.signal.aborted) catalogLoading.value = false; } }
async function reload() { error.value = ''; await Promise.all([loadOverview(), ...(props.configurationOnly || props.mediaMode ? [] : [loadCatalog()])]); }
const message = (e: unknown) => {
  const value = e instanceof Error ? e.message : '';
  if (['ARCHIVE_RUN_RECEIPT_INVALID', 'ARCHIVE_RUN_RECEIPT_MISMATCH'].includes(value)) return t('storageGov.archive.runRequest.receiptProblem');
  if (value.startsWith('ARCHIVE_RUN_CONTEXT_')) return t('storageGov.archive.runRequest.storageProblem');
  return value || t('storageGov.requestFailed');
};
async function perform(action: () => Promise<unknown>) { if (busy.value) return; const generation = operationGeneration; operationController = new AbortController(); busy.value = true; error.value = ''; notice.value = ''; try { await action(); if (!disposed && generation === operationGeneration) await reload(); } catch (e) { if (!disposed && generation === operationGeneration) error.value = message(e); } finally { if (!disposed && generation === operationGeneration) busy.value = false; } }
function editTarget(target?: ArchiveTarget) { if(!targetCanWrite.value)return; targetOpen.value = false; const key=target?.id || 'new',cached = targetDrafts.get(key); targetBaseRevision.value=cached ? targetDraftBases.get(key)! : targetControl.value!.desired_revision; targetDraftBases.set(key,targetBaseRevision.value); Object.keys(targetForm).forEach(key => { delete (targetForm as any)[key]; }); Object.assign(targetForm, targetDefaults(), cached || target || {}); error.value = ''; targetConflict.value=false; targetOpen.value = true; }
const targetDraftBases=new Map<string,number>();
const targetDrafts = new Map<string, ArchiveTarget>();
const policyDrafts = new Map<string, { form: ArchivePolicy; budget: number }>();
watch(targetForm, () => { if (targetOpen.value) targetDrafts.set(targetForm.id || 'new', { ...targetForm }); }, { flush: 'sync' });
watch([policyForm, policyBudget], () => { if (policyOpen.value) policyDrafts.set(policyForm.id || 'new', { form: { ...policyForm }, budget: policyBudget.value }); }, { flush: 'sync' });
function rejectionStatus(e: any) { return e?.status || e?.statusCode || e?.response?.status; }
function policyReceipt(out:ArchivePolicyMutation,key:string) {
 const pending=pendingConfiguration.value;if(!pending||out.request_id!==key||out.operation_revision!==pending.revision||(pending.action!=='adopt'&&out.policy_id!==pending.policyId))throw new Error('ARCHIVE_POLICY_RESPONSE_INVALID');
 if(out.policy_id)selectedPolicyId.value=out.policy_id;pendingConfiguration.value=undefined;notice.value=t('storageGov.policyControl.saved');emit('changed');
}
async function reconcileConfiguration(){await perform(async()=>{const pending=pendingConfiguration.value;if(!pending)return;policyReceipt(await api.policyRequest(pending.key,operationController?.signal),pending.key);if(pending.draftKey){policyDrafts.delete(pending.draftKey);policyDraftBases.delete(pending.draftKey)}policyOpen.value=false;policyAdoptOpen.value=false;enableCandidate.value=undefined;gcPolicyCandidate.value=undefined;});}
async function adoptPolicies(){await perform(async()=>{if(pendingConfiguration.value||pendingTarget.value||!policyControl.value)return;const key=crypto.randomUUID();pendingConfiguration.value={key,revision:1,action:'adopt'};try{policyReceipt(await api.adoptPolicies({request_id:key,legacy_digest:policyControl.value.legacy_digest,confirm:true},operationController?.signal),key);policyAdoptOpen.value=false;}catch(e){if([400,401,403,409,422,428].includes(rejectionStatus(e)))pendingConfiguration.value=undefined;throw e;}});}
function preparePolicyStatus(policy:ArchivePolicy,action:'archive'|'gc'|'pause'){
 if(!policyCanWrite.value)return;statusAction.value=action;statusBaseRevision.value=policyControl.value!.desired_revision;const target=overview.value?.targets.find(t=>t.id===policy.target_id);statusTargetRevision.value=target?.revision||0;statusTargetFingerprint.value=target?.verified_fingerprint||'';policyConflict.value=false;error.value='';if(action==='gc')gcPolicyCandidate.value=policy;else enableCandidate.value=policy;
}
async function reloadPolicyStatus(){await perform(async()=>{const previous=gcPolicyCandidate.value||enableCandidate.value;if(!previous)return;const fresh=await api.overview(operationController?.signal),policy=fresh.policies.find(p=>p.id===previous.id);if(!policy)throw new Error(t('storageGov.policyControl.unavailable'));overview.value=fresh;preparePolicyStatus(policy,statusAction.value);});}
async function writePolicyConfiguration(input:ArchivePolicy,action:'save'|'status'){
 if(!policyCanWrite.value)throw new Error(t('storageGov.policyControl.unavailable'));const key=crypto.randomUUID();
 const change:ArchivePolicyChange={request_id:key,expected_revision:action==='save'?policyBaseRevision.value:statusBaseRevision.value,expected_policy_revision:input.revision||0,policy_id:input.id||'',action};
 if(action==='save'){const{name,source_id,target_id,interval_minutes,retention_days,retention_count,remote_retention_days,remote_retention_count,remote_max_bytes,include_database}=input;change.settings={name:name.trim(),source_id,target_id,interval_minutes,retention_days,retention_count,remote_retention_days,remote_retention_count,remote_max_bytes,include_database};}
 else{Object.assign(change,{enabled:input.enabled,remote_cleanup_enabled:input.remote_cleanup_enabled,confirm:true,expected_target_revision:statusTargetRevision.value,expected_target_fingerprint:statusTargetFingerprint.value});}
 pendingConfiguration.value={key,revision:change.expected_revision+1,policyId:input.id||key,action,draftKey:action==='save'?(input.id||'new'):undefined};
 try{policyReceipt(await api.savePolicy(change,operationController?.signal),key);}catch(e){if([400,401,403,409,422,428].includes(rejectionStatus(e))){pendingConfiguration.value=undefined;policyConflict.value=rejectionStatus(e)===409;if(policyConflict.value)throw new Error(t('storageGov.policyControl.conflict'))}throw e;}
}
function targetReceipt(out: ArchiveTargetMutation,key:string) {
 const pending=pendingTarget.value;
 if(!pending || out.request_id!==key || out.operation_revision!==pending.revision || (pending.action==='save' && out.target_id!==pending.targetId))throw new Error('ARCHIVE_TARGET_RESPONSE_INVALID');
 pendingTarget.value=undefined;notice.value=t('storageGov.targetControl.saved');emit('changed');
}
async function reconcileTarget() {await perform(async()=>{const pending=pendingTarget.value;if(!pending)return;const out=await api.targetRequest(pending.key,operationController?.signal);targetReceipt(out,pending.key);if(pending.action==='adopt')targetAdoptOpen.value=false;else{targetDrafts.delete(pending.draftKey!);targetDraftBases.delete(pending.draftKey!);targetOpen.value=false;}});}
async function adoptTargets(){await perform(async()=>{if(pendingTarget.value||!targetControl.value)return;const key=crypto.randomUUID();pendingTarget.value={key,action:'adopt',revision:1};try{const out=await api.adoptTargets({request_id:key,legacy_digest:targetControl.value.legacy_digest,confirm:true},operationController?.signal);targetReceipt(out,key);targetAdoptOpen.value=false;}catch(e){if([400,401,403,409,422,428].includes(rejectionStatus(e)))pendingTarget.value=undefined;throw e;}});}
async function writeTarget(input: ArchiveTarget) {
 if(!targetCanWrite.value)throw new Error(t('storageGov.targetControl.unavailable'));
 const key=crypto.randomUUID(),{name,driver,endpoint,bucket,prefix,region,force_path_style,host,port,user,directory,credential_ref}=input;
 const change:ArchiveTargetChange={request_id:key,expected_revision:targetBaseRevision.value,expected_target_revision:input.revision||0,target_id:input.id||'',settings:{name:name.trim(),driver,endpoint,bucket,prefix,region,force_path_style,host,port,user,directory,credential_ref}};
 pendingTarget.value={key,action:'save',draftKey:input.id||'new',revision:change.expected_revision+1,targetId:input.id||key};
 try{targetReceipt(await api.saveTarget(change,operationController?.signal),key);}catch(e){if([400,401,403,409,422,428].includes(rejectionStatus(e))){pendingTarget.value=undefined;targetConflict.value=rejectionStatus(e)===409;if(targetConflict.value)throw new Error(t('storageGov.targetControl.conflict'));}throw e;}
}
async function loadLatestTarget() {
  await perform(async () => {
    const id = targetForm.id;
    const fresh = await api.overview(operationController?.signal);
    if (disposed || operationController?.signal.aborted) return;
    const target = fresh.targets.find(row => row.id === id);
    if (fresh.target_configuration?.authority !== 'database' || !fresh.target_configuration.ready || (id && !target)) throw new Error(t('storageGov.targetControl.unavailable'));
    overview.value = fresh;
    targetDrafts.delete(id || 'new');
    targetDraftBases.delete(id || 'new');
    editTarget(target);
  });
}
async function saveTarget() { await perform(async () => { await writeTarget({ ...targetForm }); targetDrafts.delete(targetForm.id || 'new');targetDraftBases.delete(targetForm.id||'new'); targetOpen.value = false; }); }



const policyDraftBases=new Map<string,number>();
function editPolicy(policy?:ArchivePolicy){if(!policyCanWrite.value)return;policyOpen.value=false;const key=policy?.id||'new',cached=policyDrafts.get(key);policyBaseRevision.value=cached?policyDraftBases.get(key)!:policyControl.value!.desired_revision;policyDraftBases.set(key,policyBaseRevision.value);delete policyForm.id;Object.assign(policyForm,policyDefaults(),policy||{});policyBudget.value=(policyForm.remote_max_bytes??GiB)/GiB;if(cached){Object.assign(policyForm,cached.form);policyBudget.value=cached.budget}error.value='';policyConflict.value=false;policyOpen.value=true;}
async function loadLatestPolicy(){await perform(async()=>{const id=policyForm.id,fresh=await api.overview(operationController?.signal),policy=fresh.policies.find(p=>p.id===id);if(fresh.policy_configuration?.authority!=='database'||!fresh.policy_configuration.ready||(id&&!policy))throw new Error(t('storageGov.policyControl.unavailable'));overview.value=fresh;policyDrafts.delete(id||'new');policyDraftBases.delete(id||'new');editPolicy(policy);});}
async function savePolicy(){if(!Number.isFinite(policyBudget.value)||policyBudget.value<0||policyBudget.value>1048576){error.value=t('storageGov.drafts.invalid');return;}await perform(async()=>{await writePolicyConfiguration({...policyForm,remote_max_bytes:Math.round(policyBudget.value*GiB)},'save');policyDrafts.delete(policyForm.id||'new');policyDraftBases.delete(policyForm.id||'new');policyOpen.value=false;});}
async function previewGC(policy: ArchivePolicy) { await perform(async () => { gcPreview.value = await api.previewRemoteGC(policy.id!, operationController?.signal); }); }
async function startGC() { const preview = gcPreview.value; if (!preview) return; await perform(async () => { await api.startRemoteGC(preview.id, operationController?.signal); gcPreview.value = undefined; gcRunsOpen.value = true; }); }
async function toggleGC() { const policy = gcPolicyCandidate.value; if (!policy) return; await perform(async () => { await writePolicyConfiguration({ ...policy, remote_cleanup_enabled: !policy.remote_cleanup_enabled },'status'); gcPolicyCandidate.value = undefined; }); }
async function togglePolicy() { const p = enableCandidate.value; if (!p) return; await perform(async () => { await writePolicyConfiguration({ ...p, enabled: statusAction.value==='pause'?false:!p.enabled,remote_cleanup_enabled:statusAction.value==='pause'?false:p.remote_cleanup_enabled },'status'); enableCandidate.value = undefined; }); }
function activeRunOwner(): ArchiveRunOwner | undefined {
  const session = readAuthSession();
  return session?.status === 'active' ? { sessionId: session.sessionId, tenantUuid: session.tenantUuid } : undefined;
}
function syncRunContext() {
  const next = activeRunOwner();
  if (JSON.stringify(next) !== JSON.stringify(runOwner)) {
    operationGeneration++; operationController?.abort(); busy.value = false;
    runOwner = next; resolvedRun.value = undefined; retryCandidate.value=undefined; error.value = ''; notice.value = '';
  }
  runContextReady.value = false; pendingRun.value = undefined; runContextProblem.value = '';
  if (!runOwner) { runContextProblem.value = t('storageGov.archive.runRequest.sessionRequired'); return; }
  try { pendingRun.value = readArchiveRunIntent(window.sessionStorage, runOwner); runContextReady.value = true; }
  catch { runContextProblem.value = t('storageGov.archive.runRequest.storageProblem'); }
}
function currentRunOwner(intent: ArchiveRunIntent) {
  const owner = activeRunOwner();
  return !disposed && !!owner && archiveRunOwnerKey(owner) === archiveRunOwnerKey(intent.owner);
}
function publishRunContext() { window.dispatchEvent(new Event(ARCHIVE_RUN_REQUEST_EVENT)); }
function acceptRunReceipt(intent: ArchiveRunIntent, value: unknown) {
  if (!currentRunOwner(intent)) return;
  const receipt = settleArchiveRunIntent(window.sessionStorage, intent, value);
  pendingRun.value = undefined; resolvedRun.value = receipt; publishRunContext();
}
async function runPolicy(policy: ArchivePolicy) {
  if (busy.value || overview.value?.run_request_version !== 1 || overview.value?.execution_version !== 1 || policyBlocked(policy) || !runContextReady.value || pendingRun.value || runContextProblem.value || !runOwner) return;
  await perform(async () => {
    const intent: ArchiveRunIntent = { version: 1, owner: { ...runOwner! }, request_id: crypto.randomUUID(), policy_id: policy.id!, policy_revision: policy.revision!, source_id: policy.source_id, created_at: new Date().toISOString() };
    try { pendingRun.value = beginArchiveRunIntent(window.sessionStorage, intent); }
    catch (e) { syncRunContext(); throw e; }
    publishRunContext();
    const job = await api.run(intent.policy_id, intent.request_id, operationController?.signal, intent.policy_revision);
    if (!currentRunOwner(intent)) return;
    acceptRunReceipt(intent, { request_id: intent.request_id, policy_id: job?.policy_id, job_id: job?.id, policy_revision: job?.policy_revision, source_id: job?.source_id, status: job?.status, phase: job?.phase, remote_state: job?.remote_state, created_at: job?.created_at, updated_at: job?.updated_at });
  });
}
async function reconcileRun() {
  const intent = pendingRun.value;
  if (!intent || !currentRunOwner(intent)) return;
  await perform(async () => { const out = await api.runRequest(intent.policy_id, intent.request_id, operationController?.signal); acceptRunReceipt(intent, out); });
}
async function showRunJob() { if (!resolvedRun.value) return; query.value = resolvedRun.value.job_id; page.value = 1; section.value = 'catalog'; await loadCatalog(); }
function prepareMigration(job: ArchiveJob) { migrationJob.value = job; migrationPolicy.value = ''; migrationKey = crypto.randomUUID(); }
async function importLegacy() { const target = importCandidate.value; if (!target) return; await perform(async () => { const result = await api.importLegacy(target.id!, operationController?.signal); notice.value = t('storageGov.archive.imported', result); importCandidate.value = undefined; section.value = 'catalog'; }); }
async function migrate() { const job = migrationJob.value; if (!job || !migrationPolicy.value) return; await perform(async () => { await api.migrate(job.id, migrationPolicy.value, migrationKey, operationController?.signal); migrationJob.value = undefined; section.value = 'catalog'; }); }
async function inspect(job: ArchiveJob) { await perform(async () => { const detail = await api.job(job.id, operationController?.signal); previewJob.value = detail; previewEntry.value = detail.files?.[0]?.path || ''; previewText.value = ''; previewOpen.value = true; }); }
async function readPreview() { if (!previewJob.value || !previewEntry.value) return; await perform(async () => { const result = await api.preview(previewJob.value!.id, previewEntry.value, operationController?.signal); previewText.value = result.text; previewTruncated.value = result.truncated; }); }
const events = useBackupEvents('archives', () => { propsRevision.value++; void reload(); });
onMounted(() => { syncRunContext(); window.addEventListener(AUTH_SESSION_EVENT, syncRunContext); window.addEventListener('storage', syncRunContext); window.addEventListener(ARCHIVE_RUN_REQUEST_EVENT, syncRunContext); void reload(); });
onBeforeUnmount(() => { disposed = true; emit('busy', false); emit('attention', false); window.removeEventListener(AUTH_SESSION_EVENT, syncRunContext); window.removeEventListener('storage', syncRunContext); window.removeEventListener(ARCHIVE_RUN_REQUEST_EVENT, syncRunContext); operationController?.abort(); overviewController?.abort(); catalogController?.abort(); });
</script>
