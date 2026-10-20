<template>
  <div class="space-y-6" data-testid="system-snapshots-panel">
    <UAlert
      color="info"
      :title="t('backupCenter.snapshots.scopeTitle')"
      :description="t('backupCenter.snapshots.scopeDescription')"
    />
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="operationNotice" color="success" :title="operationNotice" data-testid="snapshot-operation-success" />
    <UAlert
      v-if="overview?.scheduler_error"
      color="warning"
      :title="overview.scheduler_error"
    />
    <UAlert
      v-if="overview && !overview.ready"
      color="warning"
      :title="t('backupCenter.snapshots.notReady')"
      :description="overview.problems.join(' · ')"
    />
    <UAlert v-if="section !== 'recovery' && (recoveryAvailability.activeCount || recoveryAvailability.attentionCount)" color="warning" :title="t('backupCenter.sections.recoveryAttention')" class="mb-4"><template #actions><UButton size="sm" @click="section = 'recovery'">{{ t('backupCenter.sections.viewProgress') }}</UButton></template></UAlert>
    <BackupSectionTabs v-model="section" :items="sections" :label="t('backupCenter.snapshots.title')">
<template #points>
  <UCard>
    <template #header>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-lg font-semibold">{{ t('backupCenter.snapshots.workspace') }}</h2>
        <div class="flex flex-wrap gap-2">
          <UButton color="neutral" variant="ghost" :loading="loading" :disabled="busy" @click="refreshOverview()">{{ t('backupCenter.reload') }}</UButton>
          <UButton color="neutral" variant="outline" :disabled="busy" @click="open('import')">{{ t('backupCenter.snapshots.import') }}</UButton>
        </div>
      </div>
    </template>
    <UAlert v-if="recoveryUnavailableReason" data-testid="snapshot-recovery-availability" class="mb-4" :color="recoveryAvailability.status === 'loading' ? 'info' : 'warning'" :title="recoveryUnavailableReason">
      <template #actions><UButton size="sm" color="neutral" @click="section = 'recovery'">{{ t('backupCenter.recovery.viewAvailability') }}</UButton></template>
    </UAlert>
    <div v-if="overview?.directory" class="mb-4 rounded-lg border border-gray-200 p-3 dark:border-gray-700" data-testid="snapshot-storage">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <p class="text-xs text-muted">{{ t('backupCenter.snapshots.storageDirectory') }} <span class="ml-2">{{ t(`backupCenter.snapshots.storageSources.${overview.directory_source || 'explicit'}`) }}</span></p>
          <p class="mt-1 break-all font-mono text-sm" data-testid="snapshot-storage-path">{{ overview.directory }}</p>
        </div>
        <UButton size="sm" color="neutral" variant="ghost" @click="storageHelp = true">{{ t('backupCenter.snapshots.storageConfigure') }}</UButton>
      </div>
    </div>
    <section aria-labelledby="snapshot-policies-heading" data-testid="snapshot-policy-summary">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 id="snapshot-policies-heading" class="font-semibold">{{ t('backupCenter.snapshots.policy') }} <UBadge color="neutral" variant="subtle">{{ overview ? overview.policies.length : '—' }}</UBadge></h3>
        <UButton size="sm" color="neutral" variant="outline" :disabled="busy || loading" @click="openPolicy()">{{ t('backupCenter.snapshots.newPolicy') }}</UButton>
      </div>
      <p class="mb-3 text-sm text-muted">{{ t('backupCenter.snapshots.multiplePoliciesHint') }}</p>
      <p v-if="overview && !systemPolicy" class="mb-3 text-sm text-amber-600 dark:text-amber-400" data-testid="snapshot-policy-required">{{ t('backupCenter.snapshots.requireSystemPolicy') }}</p>
      <UAlert v-if="policySaved" color="success" :title="t('backupCenter.snapshots.policySaved')" class="mb-3" data-testid="snapshot-policy-saved" />
      <UAlert v-if="policyError" color="error" :title="policyError" class="mb-3" />
    <p v-if="loading && !overview" class="py-8 text-center">{{ t('backupCenter.snapshots.loading') }}</p>
    <div v-else-if="overview" class="max-h-64 overflow-auto">
      <table class="w-full min-w-[48rem] text-left text-sm" data-testid="snapshot-policy-table">
        <thead class="text-gray-500 dark:text-gray-400"><tr class="border-b border-gray-200 dark:border-gray-700">
          <th class="sticky left-0 z-10 min-w-28 bg-default px-3 py-3 font-medium">{{ t('backupCenter.policyName') }}</th>
          <th class="px-3 py-3 font-medium">{{ t('backupCenter.interval') }}</th>
          <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.retentionDays') }}</th>
          <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.retentionCount') }}</th>
          <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.scheduleStatus') }}</th>
          <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.nextRun') }}</th>
          <th class="px-3 py-3 text-right font-medium">{{ t('backupCenter.actions') }}</th>
        </tr></thead>
        <tbody>
          <tr v-for="row in overview.policies" :key="row.id" :data-policy-id="row.id" class="border-b border-gray-200 last:border-0 dark:border-gray-700">
            <td class="sticky left-0 z-10 bg-default px-3 py-4"><span class="block max-w-24 break-words font-medium">{{ row.name }}</span><UBadge v-if="row.is_default" class="mt-1" color="neutral" variant="subtle">{{ t('backupCenter.snapshots.defaultPolicy') }}</UBadge></td>
            <td class="whitespace-nowrap px-3 py-4">{{ frequency(row.interval_minutes) }}</td>
            <td class="whitespace-nowrap px-3 py-4">{{ t('backupCenter.snapshots.daysValue', {count:row.retention_days}) }}</td>
            <td class="whitespace-nowrap px-3 py-4">{{ t('backupCenter.snapshots.copiesValue', {count:row.retention_count}) }}</td>
            <td class="whitespace-nowrap px-3 py-4"><UBadge :color="row.enabled ? 'success' : 'neutral'" variant="subtle">{{ t(row.enabled ? 'backupCenter.snapshots.scheduleOn' : 'backupCenter.snapshots.scheduleOff') }}</UBadge></td>
            <td class="whitespace-nowrap px-3 py-4">{{ row.enabled && row.next_run_at ? time(row.next_run_at) : '—' }}</td>
            <td class="px-3 py-4"><div class="flex flex-col items-end justify-end gap-2 sm:flex-row">
              <UButton size="sm" class="shrink-0 whitespace-nowrap" color="neutral" variant="outline" :disabled="busy || loading" @click="openPolicy(row)">{{ t('backupCenter.snapshots.editPolicy') }}</UButton>
              <UButton v-if="!row.is_default" size="sm" class="shrink-0 whitespace-nowrap" color="neutral" variant="outline" :disabled="busy || loading" @click="selectSystemPolicy(row)">{{ t('backupCenter.snapshots.selectDefault') }}</UButton>
              <UButton v-else size="sm" class="shrink-0 whitespace-nowrap" color="neutral" variant="outline" :disabled="busy || loading || (!row.enabled && !overview.ready)" @click="setPolicyStatus(row, !row.enabled)">{{ t(row.enabled ? 'backupCenter.snapshots.disableSchedule' : 'backupCenter.snapshots.enableSchedule') }}</UButton>
              <UButton v-if="row.is_default" size="sm" class="shrink-0 whitespace-nowrap" :disabled="busy || loading || !overview.ready" @click="openCapture(row)">{{ t('backupCenter.snapshots.create') }}</UButton>
            </div></td>
          </tr>
          <tr v-if="!overview.policies?.length"><td colspan="7" class="px-3 py-8 text-center text-gray-500 dark:text-gray-400">{{ t('backupCenter.snapshots.noPolicies') }}</td></tr>
        </tbody>
      </table>
    </div>

    </section>
    <section aria-labelledby="snapshot-records-heading" class="mt-5 border-t border-gray-200 pt-5 dark:border-gray-700" data-testid="snapshot-records">
      <h3 id="snapshot-records-heading" class="mb-3 font-semibold">{{ t('backupCenter.snapshots.records') }} <UBadge color="neutral" variant="subtle">{{ overview ? overview.snapshots.length : '—' }}</UBadge></h3>
      <p v-if="loading && !overview" class="py-6 text-center text-muted">{{ t('backupCenter.snapshots.loading') }}</p>
      <div v-else-if="overview" class="overflow-x-auto">
        <table class="block w-full text-left text-sm sm:table sm:min-w-[48rem]" data-testid="snapshot-records-table">
          <thead class="hidden text-muted sm:table-header-group">
            <tr class="border-b border-gray-200 dark:border-gray-700">
              <th class="sticky left-0 z-10 min-w-36 bg-default px-3 py-3 font-medium">{{ t('backupCenter.snapshots.recordTime') }}</th>
              <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.recordSource') }}</th>
              <th class="px-3 py-3 font-medium">{{ t('backupCenter.snapshots.policyStatus') }}</th>
              <th class="px-3 py-3 font-medium">{{ t('backupCenter.size') }}</th>
              <th class="px-3 py-3 text-right font-medium">{{ t('backupCenter.actions') }}</th>
            </tr>
          </thead>
          <tbody class="block sm:table-row-group">
            <tr v-for="snapshot in overview?.snapshots" :key="snapshot.id" class="grid grid-cols-2 gap-x-3 border-b border-gray-200 py-3 last:border-0 sm:table-row sm:py-0 dark:border-gray-700" data-testid="system-snapshot">
              <td class="col-span-2 block bg-default py-2 sm:sticky sm:left-0 sm:z-10 sm:table-cell sm:px-3 sm:py-3">
                <span class="mb-1 block text-xs text-muted sm:hidden">{{ t('backupCenter.snapshots.recordTime') }}</span>
                <p class="font-medium">{{ time(snapshot.captured_at || snapshot.created_at) }}</p>
                <p class="mt-1 text-xs text-muted">{{ snapshot.instance }} · {{ snapshot.version }}</p>
                <p class="mt-1 font-mono text-xs text-muted" :title="snapshot.id">#{{ snapshot.id.slice(0, 8) }}</p>
              </td>
              <td class="block min-w-0 py-2 sm:table-cell sm:px-3 sm:py-3">
                <span class="mb-1 block text-xs text-muted sm:hidden">{{ t('backupCenter.snapshots.recordSource') }}</span>
                <p>{{ snapshot.policy_name || t('backupCenter.snapshots.noAssociatedPolicy') }}</p>
                <p class="mt-1 text-xs text-muted">{{ t(`backupCenter.snapshots.triggers.${snapshot.trigger}`) }}</p>
              </td>
              <td class="block min-w-0 py-2 sm:table-cell sm:px-3 sm:py-3">
                <span class="mb-1 block text-xs text-muted sm:hidden">{{ t('backupCenter.snapshots.policyStatus') }}</span>
                <UBadge :color="snapshot.status === 'ready' ? 'success' : snapshot.status === 'failed' || snapshot.status === 'interrupted' ? 'error' : 'neutral'" variant="subtle">{{ snapshot.phase === 'retry_wait' ? t('backupCenter.snapshots.retryWaiting') : state(snapshot.status) }}</UBadge>
                <UBadge v-if="snapshot.protected" class="ml-2" color="warning" variant="subtle" :title="t('backupCenter.snapshots.deleteProtectedHint')">{{ t('backupCenter.snapshots.protectedLabel') }}</UBadge>
                <p v-if="snapshot.error" class="mt-2 max-w-64 break-words text-sm" :class="snapshot.phase === 'retry_wait' ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400'">{{ snapshot.error }}</p>
                <p v-if="snapshot.retry_at" class="mt-1 text-xs text-muted">{{ t('backupCenter.snapshots.retryAt', { time: time(snapshot.retry_at), attempt: snapshot.attempt || 1 }) }}</p>
                <p v-if="snapshot.status === 'ready' && !snapshot.recovery_supported" class="mt-1 text-xs text-muted">{{ t('backupCenter.snapshots.onlinePackageScope') }}</p>
              </td>
              <td class="col-span-2 block whitespace-nowrap py-2 sm:table-cell sm:px-3 sm:py-3"><span class="mr-2 text-xs text-muted sm:hidden">{{ t('backupCenter.size') }}</span>{{ snapshot.components?.length ? bytes(snapshot.components.reduce((total, component) => total + component.size_bytes, 0)) : '—' }}</td>
              <td class="col-span-2 block py-2 sm:table-cell sm:px-3 sm:py-3">
                <div class="flex flex-wrap items-center justify-end gap-2">
                  <UButton size="sm" class="shrink-0 whitespace-nowrap" color="neutral" variant="ghost" @click="details = snapshot">{{ t('backupCenter.snapshots.details') }}</UButton>
                  <UButton v-if="snapshot.status === 'ready'" size="sm" class="shrink-0 whitespace-nowrap" color="neutral" variant="outline" :disabled="busy" @click="open('export', snapshot)">{{ t('backupCenter.snapshots.export') }}</UButton>
                  <UButton v-if="snapshot.status === 'ready'" size="sm" class="shrink-0 whitespace-nowrap" :disabled="busy || !canRestoreSnapshot(snapshot)" :title="snapshotRestoreReason(snapshot) || undefined" @click="restoreSnapshot(snapshot)">{{ t('backupCenter.recovery.prepare') }}</UButton>
                  <UButton size="sm" class="shrink-0 whitespace-nowrap" color="error" variant="ghost" icon="i-heroicons-trash" :disabled="busy || !!snapshotDeleteReason(snapshot)" :title="snapshotDeleteReason(snapshot) || undefined" data-testid="snapshot-delete" @click="open('delete', snapshot)">{{ t('backupCenter.snapshots.delete') }}</UButton>
                </div>
                <p v-if="snapshot.status === 'ready' && recoveryAvailability.status === 'ready' && !canRestoreSnapshot(snapshot)" class="mt-2 max-w-72 text-sm text-muted sm:ml-auto sm:text-right">{{ snapshotRestoreReason(snapshot) }}</p>
              </td>
            </tr>
            <tr v-if="!overview?.snapshots.length" class="block sm:table-row"><td colspan="5" class="block px-3 py-8 text-center text-muted sm:table-cell">{{ t('backupCenter.snapshots.empty') }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  </UCard>
</template>
<template #recovery>
<SystemRecoveryPanel ref="recoveryPanel" :snapshots="overview?.snapshots || emptySnapshots" @availability="recoveryAvailability = $event" /><UAlert v-if="!recoveryAvailability.configured" :color="recoveryAvailability.status === 'loading' ? 'info' : 'warning'" :title="recoveryUnavailableReason" />
</template>
<template #copies>
<UCard
      ><template #header
        ><h2 class="text-lg font-semibold">
          {{ t("backupCenter.snapshots.restoreHistory") }}
        </h2></template
      >
      <p v-if="!overview?.restores.length" class="py-6 text-sm text-muted">{{ t('backupCenter.sections.emptyRestores') }}</p><div class="space-y-4">
        <article
          v-for="restore in overview?.restores"
          :key="restore.id"
          class="rounded-xl border border-gray-200 p-4 dark:border-gray-700"
          data-testid="system-snapshot-restore"
        >
          <div class="flex flex-wrap justify-between gap-3">
            <p>{{ time(restore.created_at) }}</p>
            <UBadge
              :color="
                restore.status === 'prepared'
                  ? 'info'
                  : restore.status === 'failed'
                    ? 'error'
                    : 'neutral'
              "
              >{{ state(restore.status) }}</UBadge
            >
          </div>
          <p
            v-if="restore.error"
            class="mt-2 text-sm text-red-600 dark:text-red-400"
          >
            {{ restore.error }}
          </p>
          <p
            v-for="warning in restore.warnings"
            :key="warning"
            class="mt-2 text-sm text-gray-500 dark:text-gray-400"
          >
            {{ warning }}
          </p>
          <details class="mt-3 text-sm">
            <summary class="cursor-pointer">
              {{ t("backupCenter.snapshots.resourceDetails") }}
            </summary>
            <dl class="mt-3 space-y-2">
              <div>
                <dt>{{ t("backupCenter.snapshots.database") }}</dt>
                <dd class="break-all font-mono">
                  {{ restore.database || "—" }}
                </dd>
              </div>
              <div>
                <dt>{{ t("backupCenter.snapshots.directory") }}</dt>
                <dd class="break-all font-mono">{{ restore.directory }}</dd>
              </div>
            </dl>
          </details>
        </article>
      </div></UCard
    >
</template>
<template #overview>
<div class="grid gap-4 sm:grid-cols-3">
      <UCard
        ><p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t("backupCenter.snapshots.latest") }}
        </p>
        <p class="mt-2 font-semibold">
          {{
            latest
              ? time(latest.captured_at || latest.created_at)
              : t("backupCenter.snapshots.none")
          }}
        </p></UCard
      >
      <UCard
        ><p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t("backupCenter.snapshots.schedule") }}
        </p>
        <p class="mt-2 font-semibold">
          {{
            overview?.policy.enabled
              ? policyFrequency
              : t("backupCenter.snapshots.scheduleOff")
          }}
        </p></UCard
      >
      <UCard
        ><p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t("backupCenter.snapshots.nextRun") }}
        </p>
        <p class="mt-2 font-semibold">
          {{ overview?.next_run_at ? time(overview.next_run_at) : "—" }}
        </p></UCard
      >
    </div>
</template>
</BackupSectionTabs>

    <UModal
      :open="!!dialog"
      :title="dialogTitle"
      :description="dialogDescription"
      :ui="{ content: 'max-w-2xl' }"
      @update:open="
        (value) => {
          if (!value) close();
        }
      "
      ><template #body>
        <UForm
          id="system-snapshot-form"
          :state="policy"
          :schema="dialog?.kind === 'policy' ? policySchema : undefined"
          class="space-y-5"
          @submit="submit"
        >
          <div v-if="dialog?.kind === 'capture'" class="space-y-3">
            <div class="rounded-lg border border-default p-3 text-sm" data-testid="snapshot-capture-policy">
              <p class="font-semibold">{{ t('backupCenter.snapshots.captureUsingPolicy', { name: dialog.policy?.name }) }}</p>
              <p class="mt-1 text-muted">{{ frequency(dialog.policy?.interval_minutes) }} · {{ t('backupCenter.snapshots.retentionSummary', { days: dialog.policy?.retention_days, count: dialog.policy?.retention_count }) }}</p>
              <p class="mt-2 text-muted">{{ t('backupCenter.snapshots.manualCaptureHint') }}</p>
            </div>
            <UAlert v-if="!capturePolicyIsCurrent" color="warning" :title="t('backupCenter.snapshots.capturePolicyChanged')" />
            <UAlert v-else-if="overview && !overview.ready" color="warning" :title="overview.problems.join(' · ')" />
          </div>
          <template v-if="dialog?.kind === 'policy'">
            <UFormField name="name" :label="t('backupCenter.policyName')">
              <UInput v-model="policy.name" :maxlength="100" class="w-full" />
            </UFormField>
            <UFormField
              name="interval_minutes"
              :label="t('backupCenter.snapshots.interval')"
              ><UInput
                v-model.number="policy.interval_minutes"
                type="number"
                min="1"
                class="w-full"
            /></UFormField>
            <div class="grid gap-4 sm:grid-cols-2">
              <UFormField
                name="retention_days"
                :label="t('backupCenter.snapshots.retentionDays')"
                ><UInput
                  v-model.number="policy.retention_days"
                  type="number"
                  min="1"
                  class="w-full" /></UFormField
              ><UFormField
                name="retention_count"
                :label="t('backupCenter.snapshots.retentionCount')"
                ><UInput
                  v-model.number="policy.retention_count"
                  type="number"
                  min="1"
                  class="w-full"
              /></UFormField>
            </div>
            <UCheckbox
              v-model="policy.enabled"
              :label="t('backupCenter.snapshots.enable')"
            />
            <p v-if="policy.enabled" class="text-sm text-amber-600 dark:text-amber-400">{{ t('backupCenter.snapshots.defaultSwitchHint') }}</p>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("backupCenter.snapshots.captureHint") }}
            </p></template
          >
          <template v-if="dialog?.kind === 'import'"
            ><UFormField :label="t('backupCenter.snapshots.file')"
              ><input
                ref="fileInput"
                type="file"
                accept=".pxsnapshot"
                class="block w-full text-sm"
                @change="selectFile" /></UFormField
          ></template>
          <template
            v-if="dialog?.kind === 'export' || dialog?.kind === 'import'"
            ><UFormField :label="t('backupCenter.snapshots.password')"
              ><SecretInput
                v-model="password"
                autocomplete="new-password"
                class="w-full"
            /></UFormField>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("backupCenter.snapshots.passwordHint") }}
            </p></template
          >
          <p v-if="dialog?.snapshot" class="font-medium">
            {{
              time(dialog.snapshot.captured_at || dialog.snapshot.created_at)
            }}
            · {{ dialog.snapshot.version }}
          </p>
          <template v-if="dialog?.kind === 'delete' && dialog.snapshot">
            <p class="break-all font-mono text-xs text-muted">{{ dialog.snapshot.id }}</p>
            <UAlert v-if="dialog.snapshot.protected" color="warning" :title="t('backupCenter.snapshots.deleteProtectedHint')" />
            <p class="text-sm text-muted">{{ t('backupCenter.snapshots.deleteHistoryHint') }}</p>
            <UCheckbox v-model="confirmed" :label="t(dialog.snapshot.protected ? 'backupCenter.snapshots.deleteProtectedConsent' : 'backupCenter.snapshots.deleteConsent')" />
          </template>
          <UCheckbox
            v-if="dialog?.kind === 'restore'"
            v-model="confirmed"
            :label="t('backupCenter.snapshots.restoreConsent')"
          />
          <p v-if="modalError" class="text-sm text-red-600 dark:text-red-400">
            {{ modalError }}
          </p>
        </UForm></template
      ><template #footer
        ><div class="flex w-full justify-end gap-2">
          <UButton
            color="neutral"
            variant="ghost"
            :disabled="busy"
            @click="close()"
            >{{ t('backupCenter.cancel') }}</UButton
          ><UButton
              type="submit"
            form="system-snapshot-form"
            :color="dialog?.kind === 'delete' ? 'error' : 'primary'"
            :loading="busy"
            :disabled="!canSubmit"
            >{{ t(dialog?.kind === 'policy' ? 'backupCenter.save' : dialog?.kind === 'delete' ? 'backupCenter.snapshots.deleteConfirm' : 'backupCenter.confirm') }}</UButton
          >
        </div></template
      ></UModal
    >
    <UModal v-model:open="storageHelp" :title="t('backupCenter.snapshots.storageConfigure')" :description="t('backupCenter.snapshots.storageHelpHint')">
      <template #body><div class="space-y-4" data-testid="snapshot-storage-help">
        <p class="break-all font-mono text-sm">{{ overview?.directory }}</p>
        <p class="text-sm">{{ t('backupCenter.snapshots.storageYAMLHint') }}</p>
        <pre class="overflow-x-auto rounded-lg bg-muted p-3 text-xs">system_snapshots:
  directory: "/absolute/path/to/system-snapshots"</pre>
        <p class="text-sm">{{ t('backupCenter.snapshots.storageEnvironmentHint') }} <code class="break-all">POWERX_SNAPSHOT_DIRECTORY</code></p>
        <p class="text-sm">{{ t('backupCenter.snapshots.storageProfileHint') }} <code>snapshot_directory</code></p>
        <p class="text-sm text-muted">{{ t('backupCenter.snapshots.storageMoveHint') }}</p>
      </div></template>
    </UModal>
    <UModal
      :open="!!details"
      :title="t('backupCenter.snapshots.details')"
      :description="details?.id || ''"
      @update:open="
        (value) => {
          if (!value) details = null;
        }
      "
      ><template #body
        ><div v-if="details" class="space-y-4">
          <p class="font-medium">{{ state(details.status) }}</p>
          <UAlert v-if="details.error" :color="details.phase === 'retry_wait' ? 'warning' : 'error'" :title="details.error" />
          <p>
            {{ t("backupCenter.snapshots.businessVerification") }}:
            {{
              t(
                details.business_verified
                  ? "backupCenter.snapshots.verified"
                  : "backupCenter.snapshots.notVerified",
              )
            }}
          </p>
          <dl class="space-y-3">
            <div
              v-for="component in details.components"
              :key="component.kind"
              class="flex flex-wrap justify-between gap-2"
            >
              <dt>
                {{ t(`backupCenter.snapshots.components.${component.kind}`) }}
              </dt>
              <dd>
                {{
                  t("backupCenter.snapshots.files", {
                    count: component.file_count,
                  })
                }}
                · {{ bytes(component.size_bytes) }}
              </dd>
            </div>
          </dl>
          <p v-if="details.trace_id" class="break-all font-mono text-xs">
            Trace: {{ details.trace_id }}
          </p>
          <UButton
            v-if="details.status === 'ready'"
            color="neutral"
            variant="outline"
            :disabled="busy"
            @click="toggleProtection(details)"
            >{{
              t(
                details.protected
                  ? "backupCenter.unprotect"
                  : "backupCenter.protect",
              )
            }}</UButton
          >
        </div></template
      ></UModal
    >
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useBackupEvents } from "~/composables/useBackupEvents";
import * as v from "valibot";
import BackupSectionTabs from './BackupSectionTabs.vue';
 import SystemRecoveryPanel from "./SystemRecoveryPanel.vue";
import {
  useSystemSnapshotService,
  type SystemSnapshot,
  type SystemSnapshotOverview,
  type SystemSnapshotPolicyInput,
  type NamedSnapshotPolicy,
} from "~/composables/api/services/systemSnapshotService";
import { formatBackupTime } from "~/utils/backup-display";
import type { RecoveryAvailability } from '~/composables/api/services/systemRecoveryService';
const { t, locale } = useI18n();
const service = useSystemSnapshotService();
const section = ref('points');
const sections = computed(() => [
  {value:'points',label:t('backupCenter.snapshots.workspace'),icon:'i-heroicons-camera'},
  {value:'recovery',label:t('backupCenter.recovery.title'),icon:'i-heroicons-arrow-path',count:(recoveryAvailability.value.activeCount||0)+(recoveryAvailability.value.attentionCount||0)},
  {value:'copies',label:t('backupCenter.snapshots.restoreHistory'),icon:'i-heroicons-document-duplicate'},
  {value:'overview',label:t('backupCenter.sections.overview'),icon:'i-heroicons-chart-bar'},
]);
const overview = ref<SystemSnapshotOverview>();
const emptySnapshots: SystemSnapshot[] = [];
const systemPolicy = computed(() => overview.value?.policies.find(row => row.is_default && row.id === overview.value?.default_policy_id));
const recoveryAvailability = ref<RecoveryAvailability>({ configured: false, online: false, activeCount: 0, attentionCount: 0, status: 'loading', readySnapshotIDs: [] });
const recoveryUnavailableReason = computed(() => recoveryAvailability.value.status === 'ready' ? '' : (recoveryAvailability.value.reason || t(`backupCenter.recovery.availability.${recoveryAvailability.value.status}`)));
function canRestoreSnapshot(snapshot: SystemSnapshot) {
  return recoveryAvailability.value.status === 'ready' && recoveryAvailability.value.readySnapshotIDs.includes(snapshot.id);
}
function snapshotRestoreReason(snapshot: SystemSnapshot) {
  return recoveryUnavailableReason.value || (canRestoreSnapshot(snapshot) ? '' : t('backupCenter.recovery.unsupportedSnapshot'));
}
 const recoveryPanel=ref<InstanceType<typeof SystemRecoveryPanel>|null>(null);
const error = ref("");
const operationNotice = ref("");
const storageHelp = ref(false);
const modalError = ref("");
const policySaved = ref(false);
const policyError = ref("");
const busy = ref(false);
const loading = ref(false);
const password = ref("");
const confirmed = ref(false);
const selectedFile = ref<File>();
const details = ref<SystemSnapshot | null>(null);
const editingPolicyID = ref<string>();
const policy = reactive<SystemSnapshotPolicyInput>({
  name: "",
  enabled: false,
  interval_minutes: 1440,
  retention_days: 7,
  retention_count: 7,
});
const policySchema = v.object({
  name: v.pipe(v.string(), v.trim(), v.minLength(1), v.maxLength(100)),
  enabled: v.boolean(),
  interval_minutes: v.pipe(
    v.number(),
    v.integer(),
    v.minValue(1),
    v.maxValue(525600),
  ),
  retention_days: v.pipe(
    v.number(),
    v.integer(),
    v.minValue(1),
    v.maxValue(3650),
  ),
  retention_count: v.pipe(
    v.number(),
    v.integer(),
    v.minValue(1),
    v.maxValue(10000),
  ),
});
type DialogKind = "capture" | "restore" | "export" | "import" | "policy" | "delete";
const dialog = ref<{
  kind: DialogKind;
  policy?: NamedSnapshotPolicy;
  snapshot?: SystemSnapshot;
  key: string;
} | null>(null);
const capturePolicyIsCurrent = computed(() => !!dialog.value?.policy && dialog.value.policy.id === systemPolicy.value?.id);
let disposed = false;
let revision = 0;
const latest = computed(() =>
  overview.value?.snapshots.find((snapshot) => snapshot.status === "ready"),
);
const time = (value: string) =>
  formatBackupTime(value, String(locale.value || "zh"));
const bytes = (value: number) =>
  value < 1048576
    ? `${(value / 1024).toFixed(1)} KB`
    : `${(value / 1048576).toFixed(1)} MB`;
const state = (value: string) => t(`backupCenter.snapshots.states.${value}`);
function frequency(minutes?: number) {
  if (!minutes) return '—';
  if (minutes === 1440) return t('backupCenter.snapshots.daily');
  if (minutes % 1440 === 0) return t('backupCenter.snapshots.everyDays', { count: minutes / 1440 });
  if (minutes % 60 === 0) return t('backupCenter.snapshots.everyHours', { count: minutes / 60 });
  return t('backupCenter.snapshots.everyMinutes', { count: minutes });
}
const policyFrequency = computed(() => frequency(overview.value?.policy.interval_minutes));
const dialogTitle = computed(() =>
  t(
    `backupCenter.snapshots.${dialog.value?.kind === "capture" ? "create" : dialog.value?.kind === 'policy' ? (editingPolicyID.value ? 'editPolicy' : 'newPolicy') : dialog.value?.kind || "details"}`,
  ),
);
const dialogDescription = computed(() =>
  t(
    dialog.value?.kind === 'delete'
      ? 'backupCenter.snapshots.deleteHint'
      : dialog.value?.kind === "restore"
      ? "backupCenter.snapshots.restoreHint"
      : dialog.value?.kind === "capture"
        ? "backupCenter.snapshots.captureHint"
        : dialog.value?.kind === "policy"
          ? "backupCenter.snapshots.policyHint"
          : "backupCenter.snapshots.packageHint",
  ),
);
const canSubmit = computed(() => {
  if (!dialog.value || busy.value) return false;
  const kind = dialog.value.kind;
  if (kind === "capture") return capturePolicyIsCurrent.value && !!overview.value?.ready;
  if (kind === "restore") return confirmed.value;
  if (kind === 'delete') return confirmed.value && !!dialog.value.snapshot && !snapshotDeleteReason(dialog.value.snapshot);
  if (kind === "export" || kind === "import")
    return (
      new TextEncoder().encode(password.value).length >= 12 &&
      new TextEncoder().encode(password.value).length <= 1024 &&
      (kind !== "import" || !!selectedFile.value)
    );
  return (
    policy.name.trim().length > 0 && policy.name.trim().length <= 100 &&
    Number.isInteger(policy.interval_minutes) &&
    policy.interval_minutes > 0 &&
    Number.isInteger(policy.retention_days) &&
    policy.retention_days > 0 &&
    policy.retention_days * 1440 >= policy.interval_minutes &&
    Number.isInteger(policy.retention_count) &&
    policy.retention_count > 0
  );
});
function failure(e: unknown) {
  return String(
    (e as any)?.data?.message ||
      (e as any)?.message ||
      t("backupCenter.snapshots.failed"),
  );
}
async function restoreSnapshot(snapshot: SystemSnapshot) {
  if (busy.value || !canRestoreSnapshot(snapshot)) return;
  section.value = 'recovery'; await nextTick();
  if (canRestoreSnapshot(snapshot)) recoveryPanel.value?.openForSnapshot(snapshot.id);
}
async function refreshOverview() {
  await Promise.all([load(), recoveryPanel.value?.refresh()]);
}
function openDataOnly(snapshot: SystemSnapshot) { details.value=null;open('restore',snapshot); }
function open(kind: DialogKind, snapshot?: SystemSnapshot) {
  if (busy.value) return;
  if (kind === 'delete' && (!snapshot || snapshotDeleteReason(snapshot))) return;
  operationNotice.value = '';
  if (kind === 'policy') { policySaved.value = false; policyError.value = ''; }
  revision++;
  password.value = "";
  confirmed.value = false;
  selectedFile.value = undefined;
  modalError.value = "";
  dialog.value = { kind, snapshot, key: crypto.randomUUID() };
}
function openPolicy(row?: NamedSnapshotPolicy) {
  if (busy.value) return;
  open('policy');
  editingPolicyID.value = row?.id;
  Object.assign(policy, {
    name: row?.name || '', enabled: row?.enabled || false,
    interval_minutes: row?.interval_minutes || 1440,
    retention_days: row?.retention_days || 7, retention_count: row?.retention_count || 7,
  });
}
function openCapture(row: NamedSnapshotPolicy) {
  if (busy.value || row.id !== systemPolicy.value?.id || !overview.value?.ready) return;
  open('capture');
  if (dialog.value) dialog.value.policy = { ...row };
}
async function setPolicyStatus(row: NamedSnapshotPolicy, enabled: boolean) {
  if (busy.value) return;
  busy.value = true;
  policyError.value = '';
  policySaved.value = false;
  try {
    await service.setPolicyEnabled(row.id, enabled);
    if (!disposed) policySaved.value = true;
  } catch (e) {
    if (!disposed) policyError.value = failure(e);
  } finally {
    busy.value = false;
    if (!disposed) await load();
  }
}
async function selectSystemPolicy(row: NamedSnapshotPolicy) {
  if (busy.value) return;
  busy.value = true;
  policyError.value = '';
  policySaved.value = false;
  try {
    await service.setDefaultPolicy(row.id);
    if (!disposed) policySaved.value = true;
  } catch (e) {
    if (!disposed) policyError.value = failure(e);
  } finally {
    busy.value = false;
    if (!disposed) await load();
  }
}
function close() {
  if (busy.value) return;
  revision++;
  dialog.value = null;
  password.value = "";
  selectedFile.value = undefined;
  modalError.value = "";
}
function selectFile(event: Event) {
  selectedFile.value = (event.target as HTMLInputElement).files?.[0];
}
async function load() {
  if (disposed) return;
  if (loading.value) { pendingRefresh = true; return; }
  loading.value = true;
  const requested = revision;
  try {
    const data = await service.overview();
    if (!disposed && requested === revision) {
      overview.value = data;
      if (details.value) details.value = data.snapshots.find(row => row.id === details.value?.id) || details.value;
      if (dialog.value?.kind === 'delete' && dialog.value.snapshot) {
        const latest = data.snapshots.find(row => row.id === dialog.value!.snapshot!.id);
        if (latest) {
          if (latest.protected !== dialog.value.snapshot.protected) confirmed.value = false;
          dialog.value.snapshot = latest;
        }
      }
      error.value = "";
    }
  } catch (e) {
    if (!disposed && requested === revision) error.value = failure(e);
  } finally {
    loading.value = false;
    if (pendingRefresh && !disposed) { pendingRefresh = false; void load(); }
  }
}
async function submit() {
  if (!canSubmit.value || !dialog.value) return;
  const action = dialog.value;
  const requested = revision;
  busy.value = true;
  modalError.value = "";
  try {
    if (action.kind === "capture") {
      const snapshot = await service.create(action.key, action.policy!.id);
      if (!disposed && snapshot.coordinator) section.value = 'recovery';
      if (!disposed && requested === revision && overview.value) {
        // SSE 可能先于创建响应到达，不能用旧 queued 回执覆盖已推进的任务。
        const current = overview.value.snapshots.find(row => row.id === snapshot.id);
        const latest = current && snapshot.status === 'queued' && current.status !== 'queued' ? current : snapshot;
        overview.value.snapshots = [latest, ...overview.value.snapshots.filter(row => row.id !== snapshot.id)];
      }
    } else if (action.kind === "restore" && action.snapshot) {
      const restore = await service.restore(action.snapshot.id, action.key);
      if (!disposed) section.value = 'copies';
      if (!disposed && requested === revision && overview.value)
        overview.value.restores = [
          restore,
          ...overview.value.restores.filter((row) => row.id !== restore.id),
        ];
    } else if (action.kind === "export" && action.snapshot) {
      const blob = await service.exportFile(action.snapshot.id, password.value);
      if (!disposed && requested === revision) {
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = `powerx-${action.snapshot.id}.pxsnapshot`;
        link.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
    } else if (action.kind === "import" && selectedFile.value) {
      await service.importFile(selectedFile.value, password.value);
    } else if (action.kind === 'delete' && action.snapshot) {
      const result = await service.remove(action.snapshot.id, !!action.snapshot.protected && confirmed.value);
      if (!result.record_deleted || !result.files_deleted) throw new Error(t('backupCenter.snapshots.deleteIncomplete'));
      if (!disposed && requested === revision) {
        if (overview.value) overview.value.snapshots = overview.value.snapshots.filter(row => row.id !== action.snapshot!.id);
        if (details.value?.id === action.snapshot.id) details.value = null;
        operationNotice.value = t('backupCenter.snapshots.deleteSuccess');
      }
    } else if (action.kind === "policy") {
      await service.savePolicy({ ...policy, name: policy.name.trim() }, editingPolicyID.value);
      if (!disposed && requested === revision) {
        policySaved.value = true;
      }
    }
    if (!disposed && requested === revision) {
      dialog.value = null;
      password.value = "";
      selectedFile.value = undefined;
      revision++;
    }
  } catch (e) {
    if (!disposed && requested === revision) modalError.value = failure(e);
  } finally {
    busy.value = false;
    if (!disposed) await load();
  }
}
function snapshotDeleteReason(snapshot: SystemSnapshot) {
  if (!['ready', 'failed', 'interrupted', 'pruned', 'deleting', 'delete_failed'].includes(snapshot.status)) return t('backupCenter.snapshots.deleteActiveHint');
  if (recoveryAvailability.value.activeCount || recoveryAvailability.value.attentionCount) return t('backupCenter.snapshots.deleteRecoveryHint');
  return '';
}
async function toggleProtection(snapshot: SystemSnapshot) {
  if (busy.value) return;
  busy.value = true;
  try {
    const updated = await service.protect(snapshot.id, !snapshot.protected);
    if (!disposed) {
      details.value = updated;
      revision++;
    }
  } catch (e) {
    if (!disposed) error.value = failure(e);
  } finally {
    busy.value = false;
    if (!disposed) await load();
  }
}
defineExpose({ refresh: refreshOverview });
let pendingRefresh = false;
useBackupEvents("snapshots", () => { void load(); });
onMounted(load);
onBeforeUnmount(() => {
  disposed = true;
  revision++;
  password.value = "";
  selectedFile.value = undefined;
});
</script>
