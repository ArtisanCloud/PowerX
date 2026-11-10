<template>
  <section class="space-y-4" data-testid="retain-review">
    <div><h3 class="font-semibold">{{ t('backupRetain.title') }}</h3><p class="mt-2 text-sm text-muted">{{ t('backupRetain.hint') }}</p></div>
    <UAlert v-if="item && !item.review_version" color="warning" variant="subtle" :title="t('backupRetain.reviewUnavailable')" data-testid="retain-review-unavailable" />
    <UAlert v-if="error && error !== t('backupRetain.unknown')" color="warning" variant="subtle" :title="error" />
    <UAlert v-if="unknown" color="warning" variant="subtle" :title="t('backupRetain.unknown')" />
    <div v-if="receipt && (receipt.state !== 'checking' || !pending)" class="space-y-2 rounded-xl border border-default p-3" data-testid="retain-receipt">
      <UBadge color="neutral" variant="subtle">{{ t(receipt.record_source === 'maintenance' ? 'backupRetain.independentRecord' : 'backupRetain.databaseRecord') }}</UBadge>
      <UBadge :color="receipt.state === 'retained' ? 'success' : 'warning'">{{ t('backupRetain.states.' + (receipt.state === 'checking' && receipt.recovery_blocked ? 'blocked' : receipt.state)) }}</UBadge>
      <p class="break-all font-mono text-xs text-muted">{{ receipt.request_id }}</p>
      <p v-if="receipt.completed_at" class="text-xs text-muted">{{ new Date(receipt.completed_at).toLocaleString() }}</p>
      <p class="text-sm">{{ t(receipt.state === 'checking' ? receipt.recovery_blocked ? receipt.problem === 'BACKUP_RETAIN_LEGACY_HISTORY_ONLY' ? 'backupRetain.legacyHistory' : 'backupRetain.recoveryBlocked' : 'backupRetain.checking' : receipt.state === 'retained' ? 'backupRetain.kept' : receipt.problem?.includes('PAUSE') ? 'backupRetain.pauseRequired' : 'backupRetain.unchanged') }}</p>
      <details v-if="receipt.problem" class="text-xs text-muted"><summary>{{ t('backupRetain.details') }}</summary><p class="mt-2 break-all">{{ receipt.problem }}</p></details>
    </div>
    <div v-if="running" role="status" class="space-y-2"><p class="text-sm">{{ t('backupRetain.running') }} · {{ progress?.bytes_read || 0 }} / {{ progress?.total_bytes || item?.size_bytes || 0 }} B</p><progress class="h-2 w-full accent-primary" :value="progress?.bytes_read || 0" :max="Math.max(progress?.total_bytes || item?.size_bytes || 1,1)" /></div>
    <div v-if="pending" class="space-y-3 rounded-xl border border-default p-3">
      <p class="break-all font-mono text-xs" data-testid="retain-frozen-request">{{ pending.request_id }}</p>
      <p class="text-sm text-muted">{{ t('backupRetain.originalOnly') }}</p>
      <div class="flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="running || loading" data-testid="retain-reconcile" @click="reconcile">{{ t('backupRetain.reconcile') }}</UButton><UButton v-if="unknown && (!receipt || receipt.can_resume)" color="neutral" variant="outline" :disabled="running || loading" data-testid="retain-resume" @click="submit(pending)">{{ t('backupRetain.resume') }}</UButton><UButton v-if="running" color="neutral" variant="outline" data-testid="retain-stop" @click="stop">{{ t('backupRetain.stop') }}</UButton></div>
    </div>
    <template v-else-if="!foreignActive && (!receipt || receipt.state !== 'retained')">
      <UFormField :label="t('backupRetain.reason')"><UTextarea v-model="reason" :rows="3" :maxlength="512" :disabled="disabled || loading" class="w-full" data-testid="retain-reason" /></UFormField>
      <UCheckbox v-model="confirmed" :disabled="disabled || loading" :label="t('backupRetain.confirm')" data-testid="retain-confirm" />
      <UButton :disabled="disabled || foreignActive || loading || !!error || !confirmed || [...reason.trim()].length < 3 || [...reason.trim()].length > 256 || !item?.review_version" data-testid="retain-submit" @click="begin">{{ t('backupRetain.submit') }}</UButton>
    </template>
    <UButton v-if="!running && !pending" color="neutral" variant="outline" :loading="loading" data-testid="retain-refresh" @click="load">{{ t('backupRetain.refresh') }}</UButton>
  </section>
</template>
<script setup lang="ts">
import { ref,computed,watch,onMounted,onBeforeUnmount } from 'vue';
import { useI18n } from 'vue-i18n';
import type { BackupCleanupReview,BackupCleanupVerification } from '~/composables/api/services/backupCleanupReviewService';
import { useBackupRetentionService,type BackupRetentionInput,type BackupRetentionReceipt } from '~/composables/api/services/backupRetentionService';
import { useBackupEvents } from '~/composables/useBackupEvents';
const props=defineProps<{item?:BackupCleanupReview;disabled?:boolean}>();const emit=defineEmits<{locked:[value:boolean];settled:[value:string]}>();const{t}=useI18n();const api=useBackupRetentionService();
const reason=ref(''),confirmed=ref(false),error=ref(''),unknown=ref(false),running=ref(false),loading=ref(false);
const foreignActive=computed(()=>receipt.value?.state==='checking'&&!receipt.value.can_resume&&!pending.value);
const pending=ref<BackupRetentionInput>(),receipt=ref<BackupRetentionReceipt>(),progress=ref<BackupCleanupVerification>();let controller:AbortController|undefined,listController:AbortController|undefined,reconcileController:AbortController|undefined,disposed=false;
watch([pending,running],()=>emit('locked',!!pending.value||running.value),{immediate:true});
function accept(value:BackupRetentionReceipt){receipt.value=value;if(value.state!=='checking'||value.recovery_blocked){if(value.state!=='checking')emit('settled',value.state);pending.value=undefined;unknown.value=false;running.value=false;confirmed.value=false;controller?.abort()}else if(value.can_resume&&!pending.value){pending.value={request_id:value.request_id,expected_token:value.token,review_version:value.review_version,reason:value.reason,confirm:true};unknown.value=true}}
async function load(){if(!props.item)return;listController?.abort();const c=listController=new AbortController();loading.value=true;try{const values=await api.list(props.item.artifact_id,props.item.token,c.signal);if(c.signal.aborted||disposed)return;if(pending.value){const found=values.find(v=>v.request_id===pending.value!.request_id);if(found)accept(found)}else {const current=values.find(v=>v.state==='checking')||values.find(v=>v.state==='retained')||values[0];if(current)accept(current)}error.value=''}catch{if(!c.signal.aborted&&!disposed)error.value=t('backupRetain.loadFailed')}finally{if(!c.signal.aborted&&!disposed)loading.value=false}}
function begin(){if(!props.item?.review_version||!confirmed.value||props.disabled)return;const value:BackupRetentionInput={request_id:crypto.randomUUID(),expected_token:props.item.token,review_version:props.item.review_version,reason:reason.value.trim(),confirm:true};void submit(value)}
async function submit(value:BackupRetentionInput){if(!props.item||running.value)return;pending.value={...value};running.value=true;unknown.value=false;error.value='';progress.value=undefined;const c=controller=new AbortController();try{const result=await api.retain(props.item.artifact_id,{...value},c.signal,event=>{if(c.signal.aborted||disposed)return;if(event.receipt)receipt.value=event.receipt;if(event.progress)progress.value=event.progress});if(!c.signal.aborted&&!disposed)accept(result)}catch{if(!disposed&&pending.value){unknown.value=true;error.value=t('backupRetain.unknown')}}finally{if(!disposed)running.value=false}}
async function reconcile(){if(!pending.value)return;loading.value=true;reconcileController?.abort();const c=reconcileController=new AbortController();try{const result=await api.request(pending.value.request_id,c.signal);if(!disposed&&!c.signal.aborted){accept(result);error.value=''}}catch{if(!disposed)error.value=t('backupRetain.noReceipt')}finally{if(!disposed)loading.value=false}}
function stop(){controller?.abort();running.value=false;unknown.value=true;error.value=t('backupRetain.unknown')}
useBackupEvents('backup_cleanup',()=>void load());onMounted(()=>void load());onBeforeUnmount(()=>{disposed=true;controller?.abort();listController?.abort();reconcileController?.abort();emit('locked',false)});
</script>
