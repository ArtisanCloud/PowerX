<template>
 <UModal v-model:open="open" :title="t('storageGov.runtime.budget')" :description="t('storageGov.hostBudget.hint')" :dismissible="!saving && !unknown" :close="saving || unknown ? false : undefined" :ui="{content:'max-w-4xl'}">
  <template #body><div class="min-w-0 space-y-4" data-testid="storage-runtime-budget">
   <UAlert v-if="error" color="error" :title="error" />
   <div class="rounded-lg border border-default p-3 text-sm" data-testid="budget-versions">
    <p>{{ t('storageGov.hostBudget.owner', {owner:configuration?.owner_instance || '—'}) }}</p>
    <p class="mt-2">{{ t('storageGov.tableMonitor.control.versions', {desired:configuration?.desired_revision ?? '—',applied:configuration?.applied_revision ?? '—',snapshot:configuration?.snapshot_revision ?? '—',state:t(`storageGov.tableMonitor.control.states.${configuration?.state || 'unavailable'}`)}) }}</p>
    <p v-if="configuration?.pending_protection" class="mt-2 text-warning">{{ t('storageGov.hostBudget.pending') }}</p>
    <p v-if="configuration?.authority === 'database' && !configuration.can_manage" class="mt-2 text-warning">{{ t('storageGov.hostBudget.otherOwner') }}</p>
   </div>
   <div v-if="unknown" class="space-y-3" data-testid="budget-unknown"><UAlert color="warning" :title="t('storageGov.tableMonitor.control.unknown')" :description="t('storageGov.tableMonitor.control.unknownHint')" /><UButton color="neutral" variant="outline" :loading="saving" data-testid="budget-reconcile" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton></div>
   <BackupSectionTabs v-model="section" :items="sections" :label="t('storageGov.runtime.budget')">
    <template #overview>
     <div class="space-y-4"><h3 class="font-semibold">{{ t('storageGov.hostBudget.effective') }}</h3><dl class="grid gap-3 text-sm sm:grid-cols-2"><div v-for="item in effectiveValues" :key="item.label"><dt class="text-muted">{{item.label}}</dt><dd class="mt-1 font-medium">{{item.value}}</dd></div></dl><UAlert color="info" :title="t('storageGov.hostBudget.admission')" /><UButton color="neutral" variant="outline" :loading="loading" @click="refresh">{{ t('storageGov.refresh') }}</UButton></div>
    </template>
    <template #settings>
     <div v-if="configuration?.authority === 'legacy'" class="space-y-4" data-testid="budget-adoption"><UAlert color="warning" :title="t('storageGov.hostBudget.legacy')" :description="t('storageGov.hostBudget.adoptionHint')" /><UCheckbox v-model="confirmed" :disabled="saving || !!unknown" :label="t('storageGov.hostBudget.confirmAdopt')" data-testid="budget-confirm" /><UButton :disabled="!configuration.can_manage || !confirmed || saving || !!unknown" data-testid="budget-adopt" @click="mutate(true)">{{ t('storageGov.hostBudget.adopt') }}</UButton><UButton color="neutral" variant="ghost" :disabled="saving || !!unknown" @click="loadLatest">{{ t('storageGov.tableMonitor.control.loadLatest') }}</UButton></div>
     <UAlert v-else-if="!configuration || configuration.authority !== 'database'" color="warning" :title="t('storageGov.tableMonitor.control.upgrade')" :description="t('storageGov.tableMonitor.control.unavailableHint')" />
     <UForm v-else :state="form" :schema="schema" class="space-y-4" data-testid="budget-form" @submit="mutate(false)">
      <p class="text-sm text-muted">{{ t('storageGov.hostBudget.editing', {version:baseRevision}) }}</p>
      <p v-if="baseRevision !== configuration.desired_revision" class="text-sm text-warning" data-testid="budget-stale">{{ t('storageGov.tableMonitor.control.staleDraft', {version:baseRevision}) }}</p>
      <fieldset :disabled="saving || !!unknown || !configuration.can_manage || loading" class="grid min-w-0 gap-4 sm:grid-cols-2">
       <UFormField v-for="field in fields" :key="field.key" :name="field.key" :label="t(`storageGov.runtime.${field.label}`)"><UInput v-model="form[field.key]" type="number" :min="field.key === 'maximum' ? 0 : field.key === 'concurrent' ? 1 : 1/GiB" :max="field.key === 'concurrent' ? 16 : 1048576" :step="field.key === 'concurrent' ? 1 : 'any'" class="w-full" :data-testid="`budget-${field.key}`" /></UFormField>
      </fieldset>
      <p class="text-xs text-muted">{{ t('storageGov.hostBudget.zero') }}</p><UCheckbox v-model="confirmed" :disabled="saving || !!unknown || !configuration.can_manage" :label="t('storageGov.runtime.confirmBudget')" data-testid="budget-confirm" />
      <div class="flex flex-wrap gap-2"><UButton type="submit" :loading="saving" :disabled="!configuration.can_manage || !confirmed || !!unknown || loading" data-testid="budget-save">{{ t('storageGov.runtime.save') }}</UButton><UButton type="button" color="neutral" variant="ghost" :disabled="saving || !!unknown" data-testid="budget-load-latest" @click="loadLatest">{{ t('storageGov.tableMonitor.control.loadLatest') }}</UButton></div>
     </UForm>
     <UAlert v-if="saved" class="mt-4" :color="configuration?.owner_instance===savedOwner && configuration.state==='applied' && configuration.desired_revision===baseRevision ? 'success' : 'info'" :title="t(configuration?.owner_instance!==savedOwner ? 'storageGov.hostBudget.savedOtherOwner' : configuration?.state==='applied' && configuration.desired_revision===baseRevision ? 'storageGov.tableMonitor.control.applied' : 'storageGov.hostBudget.saved')" />
    </template>
    <template #reservations><div class="space-y-3"><p class="text-sm text-muted">{{ t('storageGov.hostBudget.reservationsHint') }}</p><p v-if="!runtime?.budget?.active.length" class="text-muted">{{ t('storageGov.hostBudget.empty') }}</p><div v-for="lease in runtime?.budget?.active" :key="lease.id" class="rounded-lg border border-default p-3 text-sm"><p>{{lease.instance_id}} · {{bytes(lease.peak_bytes-lease.written_bytes)}}</p><p class="mt-1 text-muted">{{new Date(lease.expires_at).toLocaleString()}}</p></div></div></template>
   </BackupSectionTabs>
  </div></template>
 </UModal>
</template>
<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,reactive,ref,watch} from 'vue';
import {useI18n} from 'vue-i18n';
import * as v from 'valibot';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import {useStorageRuntimeService,type StorageRuntime,type HostBudgetConfiguration,type HostBudgetSettings,type HostBudgetMutation} from '~/composables/api/services/storageRuntimeService';
import {formatStorageBytes as bytes} from '~/utils/storage-capacity';
const props=defineProps<{runtime?:StorageRuntime}>(),open=defineModel<boolean>('open',{default:false});
const emit=defineEmits<{saved:[];busy:[boolean]}>(),{t}=useI18n(),api=useStorageRuntimeService(),GiB=2**30;
const section=ref('overview'),configuration=ref<HostBudgetConfiguration>(),loading=ref(false),saving=ref(false),error=ref(''),confirmed=ref(false),saved=ref(false),baseRevision=ref(0);
const form=reactive({minimum:20 as number|string,critical:10 as number|string,maximum:0 as number|string,concurrent:1 as number|string});
const fields=[{key:'minimum',label:'minimum'},{key:'critical',label:'critical'},{key:'maximum',label:'maximum'},{key:'concurrent',label:'concurrent'}] as const;
const sections=computed(()=>[{value:'overview',label:t('storageGov.hostBudget.overview')},{value:'settings',label:t('storageGov.hostBudget.settings')},{value:'reservations',label:t('storageGov.runtime.reservations')}]);
const numeric=(min:number,max:number,integer=false)=>v.pipe(v.union([v.string(),v.number()]),v.transform(Number),v.check(n=>Number.isFinite(n)&&n>=min&&n<=max&&(!integer||Number.isInteger(n)),t('storageGov.drafts.invalid')));
const schema=computed(()=>v.pipe(v.object({minimum:numeric(1/GiB,1048576),critical:numeric(1/GiB,1048576),maximum:numeric(0,1048576),concurrent:numeric(1,16,true)}),v.check(p=>p.critical<p.minimum,t('storageGov.drafts.invalid'))));
const settings=():HostBudgetSettings=>({min_free_bytes:Math.round(Number(form.minimum)*GiB),critical_free_bytes:Math.round(Number(form.critical)*GiB),max_used_bytes:Math.round(Number(form.maximum)*GiB),max_concurrent:Number(form.concurrent)});
const same=(a:HostBudgetSettings,b:HostBudgetSettings)=>a.min_free_bytes===b.min_free_bytes&&a.critical_free_bytes===b.critical_free_bytes&&a.max_used_bytes===b.max_used_bytes&&a.max_concurrent===b.max_concurrent;
const effectiveValues=computed(()=>{const p=configuration.value?.effective_policy;return [{label:t('storageGov.hostBudget.minimumLabel'),value:p?bytes(p.min_free_bytes):'—'},{label:t('storageGov.hostBudget.criticalLabel'),value:p?bytes(p.critical_free_bytes):'—'},{label:t('storageGov.hostBudget.maximumLabel'),value:p?(p.max_used_bytes?bytes(p.max_used_bytes):t('storageGov.hostBudget.noMaximum')):'—'},{label:t('storageGov.hostBudget.concurrentLabel'),value:p?.max_concurrent??'—'}]});
type Operation={owner:string;key:string;revision:number;settings:HostBudgetSettings;adopting:boolean;legacyRevision:number;legacyDigest:string};
const savedOwner=ref('');
const unknown=ref<Operation>();let origin={revision:0,digest:''},initialized=false,read:AbortController|undefined,write:AbortController|undefined;
function accept(c:HostBudgetConfiguration){const old=configuration.value;if(old?.authority==='database'&&(c.authority==='legacy'||c.desired_revision<old.desired_revision&&!['unconfirmed','unavailable'].includes(c.state))){configuration.value={...old,authority:'unavailable',state:'unconfirmed',can_manage:false,problem:'STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED'};return}configuration.value=c;}
function fill(){const c=configuration.value,p=c?.desired_policy||c?.effective_policy;if(!c||!p)return;Object.assign(form,{minimum:p.min_free_bytes/GiB,critical:p.critical_free_bytes/GiB,maximum:p.max_used_bytes/GiB,concurrent:p.max_concurrent});baseRevision.value=c.desired_revision;origin={revision:c.legacy_revision,digest:c.legacy_digest};initialized=true;confirmed.value=false;}
async function refresh(){read?.abort();const r=read=new AbortController();loading.value=true;error.value='';try{const c=await api.budgetConfiguration(r.signal);if(!r.signal.aborted)accept(c);return !r.signal.aborted;}catch(e){if(!r.signal.aborted){error.value=e instanceof Error?e.message:t('storageGov.requestFailed');if(configuration.value)configuration.value={...configuration.value,can_manage:false}}return false;}finally{if(!r.signal.aborted)loading.value=false}}
async function loadLatest(){if(saving.value||unknown.value)return;if(await refresh())fill();}
async function accepted(m:HostBudgetMutation,op:Operation){if(m.request_id!==op.key||m.operation_revision!==op.revision+1||!same(m.operation_policy,op.settings))throw new Error('STORAGE_BUDGET_RESPONSE_INVALID');accept(m.configuration);savedOwner.value=op.owner;baseRevision.value=m.operation_revision;unknown.value=undefined;confirmed.value=false;Object.assign(form,{minimum:op.settings.min_free_bytes/GiB,critical:op.settings.critical_free_bytes/GiB,maximum:op.settings.max_used_bytes/GiB,concurrent:op.settings.max_concurrent});origin={revision:m.configuration.legacy_revision,digest:m.configuration.legacy_digest};initialized=true;await nextTick();saved.value=true;emit('saved');}
async function mutate(adopting:boolean){const c=configuration.value;if(!c?.can_manage||!confirmed.value||saving.value||unknown.value||(!adopting&&!v.safeParse(schema.value,form).success))return;
 const op:Operation={owner:c.owner_instance,key:crypto.randomUUID(),revision:adopting?0:baseRevision.value,settings:adopting?{...c.effective_policy!}:settings(),adopting,legacyRevision:origin.revision,legacyDigest:origin.digest};const r=write=new AbortController();saving.value=true;error.value='';saved.value=false;
 try{const m=adopting?await api.adoptBudget({request_id:op.key,legacy_revision:op.legacyRevision,legacy_digest:op.legacyDigest,confirm:true},r.signal):await api.saveBudget({request_id:op.key,expected_revision:op.revision,settings:op.settings,confirm:true},r.signal);if(!r.signal.aborted)await accepted(m,op)}catch(e:any){if(!r.signal.aborted){error.value=e?.message||t('storageGov.requestFailed');const status=e?.status||e?.statusCode||e?.response?.status;if(![400,401,403,409,422,428].includes(status))unknown.value=op}}finally{if(!r.signal.aborted)saving.value=false}}
async function reconcile(){const op=unknown.value;if(!op||saving.value)return;const r=write=new AbortController();saving.value=true;error.value='';try{const m=await api.budgetRequest(op.key,r.signal);if(!r.signal.aborted)await accepted(m,op)}catch{if(!r.signal.aborted)error.value=t('storageGov.tableMonitor.control.unknownHint')}finally{if(!r.signal.aborted)saving.value=false}}
watch(()=>props.runtime?.budget_configuration,c=>{if(c)accept(c)},{immediate:true});
watch(open,async value=>{if(value){if(!initialized)fill();if(await refresh())if(!initialized)fill();}});
watch(form,()=>{confirmed.value=false;saved.value=false},{deep:true});
watch(()=>saving.value||!!unknown.value,b=>emit('busy',b));
onBeforeUnmount(()=>{read?.abort();write?.abort();emit('busy',false)});
</script>
