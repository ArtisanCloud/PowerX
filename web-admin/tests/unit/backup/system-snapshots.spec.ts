import { mount, flushPromises } from "@vue/test-utils";
import { ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { emitBackupEvent } from '../../support/backup-events-stub';
vi.mock('~/composables/useBackupEvents', () => import('../../support/backup-events-stub'));
const service = vi.hoisted(() => ({
  overview: vi.fn(),
  savePolicy: vi.fn(),
  setPolicyEnabled: vi.fn(),
  setDefaultPolicy: vi.fn(),
  create: vi.fn(),
  exportFile: vi.fn(),
  importFile: vi.fn(),
  restore: vi.fn(),
  protect: vi.fn(),
}));
const recoveryService = vi.hoisted(() => ({ overview: vi.fn(), restore: vi.fn(), watchProgress: vi.fn() }));
vi.mock('~/composables/api/services/systemRecoveryService', () => ({ useSystemRecoveryService: () => recoveryService }));
vi.mock("~/composables/api/services/systemSnapshotService", () => ({
  useSystemSnapshotService: () => service,
}));
import Panel from "~/components/ops/backup/SystemSnapshotsPanel.vue";
const stubs = { UIcon: true,
 SystemRecoveryPanel: { name: 'SystemRecoveryPanel', template: '<div/>', methods: { refresh() {} } },
  UCard: { template: '<div><slot name="header"/><slot/></div>' },
  UButton: {
    props: ["disabled", "loading"],
    template: '<button :disabled="disabled || loading"><slot/></button>',
  },
  UBadge: { template: "<span><slot/></span>" },
  UAlert: {
    props: ["title", "description"],
    template: '<p>{{title}} {{description}}<slot name="actions"/></p>',
  },
  UModal: {
    props: ["open"],
    emits: ["update:open"],
    template: '<div v-if="open"><slot name="body"/><slot name="footer"/></div>',
  },
  UFormField: { template: "<label><slot/></label>" },
  UInput: true,
  USelect: {
    props: ['modelValue', 'items'],
    emits: ['update:modelValue'],
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="item in items" :key="item.value" :value="item.value">{{item.label}}</option></select>',
  },
  SecretInput: true,
  UCheckbox: {
    props: ["modelValue", "label"],
    emits: ["update:modelValue"],
    template:
      '<label><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)"/>{{label}}</label>',
  },
  UForm: {
    emits: ["submit"],
    template: "<form @submit.prevent=\"$emit('submit')\"><slot/></form>",
  },
};
const snapshot = {
  id: "snapshot-1",
  created_at: "2026-10-15T00:00:00Z",
  status: "ready",
  phase: "complete",
  instance: "test",
  version: "v1",
  trigger: "manual",
  components: [{ kind: "database", file_count: 1, size_bytes: 100 }],
  business_verified: false,
};
const overview = () => ({
  directory: "/fixture/snapshot storage",
  directory_source: "configuration",
  ready: true,
  problems: [],
  instance: "test",
  policy: {
    enabled: false,
    interval_minutes: 1440,
    retention_days: 7,
    retention_count: 7,
  },
  policies: [{id:'policy-1',name:'Daily',is_default:true,enabled:false,interval_minutes:1440,retention_days:7,retention_count:7}],
  default_policy_id: 'policy-1',
  snapshots: [snapshot],
  restores: [],
  cutover_available: false,
});
const wrappers: ReturnType<typeof mount>[] = [];
const panel = () => {
  const wrapper = mount(Panel, { global: { stubs } });
  wrappers.push(wrapper);
  return wrapper;
};
const button = (w: ReturnType<typeof mount>, key: string) =>
  w
    .findAll("button")
    .find((b) => b.attributes('role') !== 'tab' && b.text() === `backupCenter.snapshots.${key}`)!;
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("useI18n", () => ({
    t: (key: string, params?: {name?: string}) => params?.name ? `${key}: ${params.name}` : key,
    te: () => true,
    locale: ref("zh"),
  }));
  service.overview.mockResolvedValue(overview());
  recoveryService.overview.mockResolvedValue({ configured: true, online: true, generation: 1, targets: [{ id: 'local', allow_overwrite: true, environment: 'dev', allow_online_data: true }], jobs: [], progress_tokens: {} });
  recoveryService.watchProgress.mockReturnValue(vi.fn());
  service.create.mockResolvedValue({ ...snapshot, status: "queued" });
  service.restore.mockResolvedValue({
    id: "restore-1",
    status: "queued",
    warnings: [],
  });
});
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
  vi.useRealTimers();
});
describe("system snapshots", () => {
  const integratedPanel = () => {
    const wrapper = mount(Panel, { global: { stubs: { ...stubs, SystemRecoveryPanel: false } } });
    wrappers.push(wrapper);
    return wrapper;
  };
  const restoreButton = (w: ReturnType<typeof mount>) => w.get('[data-testid="system-snapshot"]').findAll('button').find(b => b.text() === 'backupCenter.recovery.prepare')!;

  it('shows recovery availability while the snapshot list is still loading, without repeated updates', async () => {
    let resolve!: (value: any) => void;
    service.overview.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    recoveryService.overview.mockResolvedValue({ configured: false, online: false, generation: 0, targets: [], jobs: [], progress_tokens: {} });
    const w = integratedPanel(); await flushPromises();
    expect(w.get('[data-testid="snapshot-recovery-availability"]').text()).toContain('backupCenter.recovery.availability.unconfigured');
    expect(w.find('[data-testid="system-snapshot"]').exists()).toBe(false);
    resolve(overview()); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeDefined();
    expect(w.get('[data-testid="snapshot-records-table"]').isVisible()).toBe(true);
    expect(recoveryService.overview).toHaveBeenCalledTimes(1);
  });

  it('checks restore availability before enabling the action and explains an unconfigured site', async () => {
    let resolve!: (value: any) => void;
    recoveryService.overview.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    const w = integratedPanel(); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeDefined();
    expect(w.get('[data-testid="snapshot-recovery-availability"]').text()).toContain('backupCenter.recovery.availability.loading');
    resolve({ configured: false, online: false, generation: 0, targets: [], jobs: [], progress_tokens: {} }); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeDefined();
    expect(w.get('[data-testid="snapshot-recovery-availability"]').text()).toContain('backupCenter.recovery.availability.unconfigured');
    expect(w.get('[data-section="points"]').isVisible()).toBe(true);
    expect(button(w, 'export').attributes('disabled')).toBeUndefined();
    expect(recoveryService.restore).not.toHaveBeenCalled();
  });

  it('opens the selected snapshot confirmation on the first click when the installation is ready', async () => {
    const w = integratedPanel(); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeUndefined();
    await restoreButton(w).trigger('click'); await flushPromises();
    expect(w.get('[data-section="recovery"]').isVisible()).toBe(true);
    expect((w.get('select:not([data-section-select])').element as HTMLSelectElement).value).toBe(snapshot.id);
    expect(w.find('input[type="checkbox"]').exists()).toBe(true);
    expect(recoveryService.restore).not.toHaveBeenCalled();
  });

  it('distinguishes a failed availability request from an unconfigured installation and supports manual refresh', async () => {
    recoveryService.overview.mockRejectedValueOnce(new Error('unavailable'));
    const w = integratedPanel(); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeDefined();
    expect(w.get('[data-testid="snapshot-recovery-availability"]').text()).toContain('backupCenter.recovery.availability.error');
    await (w.vm as any).refresh(); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeUndefined();
    expect(w.find('[data-testid="snapshot-recovery-availability"]').exists()).toBe(false);
  });

  it('blocks an unsupported package on a production installation without preventing export', async () => {
    recoveryService.overview.mockResolvedValue({ configured: true, online: true, generation: 1, targets: [{ id: 'prod', allow_overwrite: true, environment: 'prod', allow_online_data: true }], jobs: [], progress_tokens: {} });
    const w = integratedPanel(); await flushPromises();
    expect(restoreButton(w).attributes('disabled')).toBeDefined();
    expect(w.get('[data-testid="system-snapshot"]').text()).toContain('backupCenter.recovery.unsupportedSnapshot');
    expect(button(w, 'export').attributes('disabled')).toBeUndefined();
    expect(recoveryService.restore).not.toHaveBeenCalled();
  });

  it.each([null, undefined])("renders an old failed snapshot with components=%s", async (components) => {
    service.overview.mockResolvedValue({ ...overview(), snapshots: [{ ...snapshot, status: 'failed', phase: 'failed', components, error: '程序产物软链接越界' }] });
    const w = panel();
    await flushPromises();
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('Daily');
    const row = w.get('[data-testid="system-snapshot"]');
    expect(row.text()).toContain('程序产物软链接越界');
    expect(row.text()).toContain('—');
    expect(w.text()).not.toContain('backupCenter.snapshots.loading');
    await button(w, 'details').trigger('click');
    expect(w.text()).toContain('程序产物软链接越界');
    expect(service.create).not.toHaveBeenCalled();
  });

  it("ends initial loading on a request error and allows manual retry", async () => {
    service.overview.mockRejectedValueOnce(new Error('snapshot request unavailable'));
    const w = panel();
    await flushPromises();
    expect(w.text()).toContain('snapshot request unavailable');
    expect(w.text()).not.toContain('backupCenter.snapshots.loading');
    expect(w.find('[data-testid="snapshot-policy-table"]').exists()).toBe(false);
    expect(w.find('[data-testid="system-snapshot"]').exists()).toBe(false);
    await w.findAll('button').find(b => b.text() === 'backupCenter.reload')!.trigger('click');
    await flushPromises();
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('Daily');
    expect(w.findAll('[data-testid="system-snapshot"]')).toHaveLength(1);
    expect(w.text()).not.toContain('snapshot request unavailable');
  });

  it("shows policies and snapshot records together and returns to both tables after saving", async () => {
    const current = overview();
    service.overview.mockResolvedValue(current);
    service.savePolicy.mockImplementation(async (saved,id) => { current.policies = [{...saved,id,is_default:true}]; return current.policies[0]; });
    const w = panel();
    await flushPromises();
    expect(w.find('[data-section="policy"]').exists()).toBe(false);
    expect(w.get('[data-testid="snapshot-policy-table"]').isVisible()).toBe(true);
    expect(w.get('[data-testid="snapshot-records-table"]').isVisible()).toBe(true);
    expect(w.get('[data-testid="system-snapshot"]').text()).toContain('#snapshot');
    await button(w, 'editPolicy').trigger('click');
    await w.get('form').trigger('submit');
    await flushPromises();
    expect(service.savePolicy).toHaveBeenCalledWith({name:'Daily',enabled:false,interval_minutes:1440,retention_days:7,retention_count:7}, 'policy-1');
    expect(w.find('form').exists()).toBe(false);
    expect(w.get('[data-testid="snapshot-policy-saved"]').isVisible()).toBe(true);
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('Daily');
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('backupCenter.snapshots.daily');
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('backupCenter.snapshots.scheduleOff');
    expect(w.findAll('[data-testid="system-snapshot"]')).toHaveLength(1);
    expect(service.create).not.toHaveBeenCalled();
  });

  it("keeps saved policies visible when there are no snapshot entities", async () => {
    service.overview.mockResolvedValue({...overview(),snapshots:[]});
    const w = panel();
    await flushPromises();
    expect(w.get('[data-testid="snapshot-policy-table"]').text()).toContain('Daily');
    expect(w.get('[data-testid="snapshot-records-table"]').text()).toContain('backupCenter.snapshots.empty');
    expect(w.findAll('[data-testid="system-snapshot"]')).toHaveLength(0);
    expect(service.create).not.toHaveBeenCalled();
  });

  it("shows policy action failures beside the policy table and preserves snapshot records", async () => {
    service.setPolicyEnabled.mockRejectedValueOnce(new Error('activation unavailable'));
    const w = panel();
    await flushPromises();
    await button(w, 'enableSchedule').trigger('click');
    await flushPromises();
    expect(w.get('[data-testid="snapshot-policy-summary"]').text()).toContain('activation unavailable');
    expect(w.get('[data-testid="snapshot-records-table"]').isVisible()).toBe(true);
    expect(w.findAll('[data-testid="system-snapshot"]')).toHaveLength(1);
    expect(service.create).not.toHaveBeenCalled();
  });

  it("renders separate snapshot entities with their captured policies and failure details", async () => {
    service.overview.mockResolvedValue({...overview(),snapshots:[
      {...snapshot,policy_id:'policy-old',policy_name:'Original policy'},
      {...snapshot,id:'failed-1',status:'failed',policy_name:'Another policy',error:'writer drain timed out',components:[]},
    ]});
    const w = panel();
    await flushPromises();
    const rows=w.findAll('[data-testid="system-snapshot"]');
    expect(rows).toHaveLength(2);
    expect(rows[0]!.text()).toContain('Original policy');
    expect(rows[1]!.text()).toContain('Another policy');
    expect(rows[1]!.text()).toContain('writer drain timed out');
    expect(rows[1]!.text()).not.toContain('backupCenter.snapshots.restore');
  });

  it("keeps active recovery discoverable outside its section and preserves the recovery instance", async () => {
    const w = mount(Panel, {attachTo:document.body,global:{stubs}}); wrappers.push(w);
    await flushPromises();
    const recovery = w.findComponent({name:'SystemRecoveryPanel'});
    recovery.vm.$emit('availability',{configured:true,online:true,activeCount:1,attentionCount:0,status:'busy',readySnapshotIDs:[]});
    await flushPromises();
    expect(w.text()).toContain('backupCenter.sections.recoveryAttention');
    await w.findAll('button').find(b=>b.text()==='backupCenter.sections.viewProgress')!.trigger('click');
    expect(w.get('[data-section="recovery"]').isVisible()).toBe(true);
    expect(w.get('[data-section="points"]').isVisible()).toBe(false);
    expect(w.findComponent({name:'SystemRecoveryPanel'}).vm).toBe(recovery.vm);
  });

  it("shows package scope and keeps paths and unverified status in details", async () => {
    const w = panel();
    await flushPromises();
    expect(w.text()).toContain("backupCenter.snapshots.scopeDescription");
    expect(w.text()).not.toContain(
      "backupCenter.snapshots.businessVerification",
    );
    await button(w, "details").trigger("click");
    expect(w.text()).toContain("backupCenter.snapshots.notVerified");
    expect(service.create).not.toHaveBeenCalled();
  });
  it("offers manual capture with scheduling off without enabling the schedule", async () => {
    const w=panel(); await flushPromises();
    expect(button(w, 'create').attributes('disabled')).toBeUndefined();
    expect(w.find('[data-testid="snapshot-policy-required"]').exists()).toBe(false);
    await button(w,'create').trigger('click');
    expect(w.get('form').text()).toContain('backupCenter.snapshots.manualCaptureHint');
    await w.get('form').trigger('submit'); await flushPromises();
    expect(service.create).toHaveBeenCalledWith(expect.any(String),'policy-1');
    expect(service.setPolicyEnabled).not.toHaveBeenCalled();
  });

  it("selects a different default independently of automatic scheduling", async () => {
    let current=overview(); current.policy.enabled=true; current.policies[0]!.enabled=true;
    current.policies.push({...current.policies[0]!,id:'policy-2',name:'Upgrade',enabled:false,is_default:false});
    service.overview.mockImplementation(async()=>current);
    service.setDefaultPolicy.mockImplementation(async(id)=> {
      current={...current,default_policy_id:id,policy:{...current.policy,enabled:false},policies:current.policies.map(p=>({...p,is_default:p.id===id,enabled:false}))};
      return current.policies.find(p=>p.id===id);
    });
    const w=panel(); await flushPromises();
    await button(w,'selectDefault').trigger('click'); await flushPromises();
    expect(service.setDefaultPolicy).toHaveBeenCalledWith('policy-2');
    expect(service.setPolicyEnabled).not.toHaveBeenCalled();
    expect(current.policies.every(p=>!p.enabled)).toBe(true);
    await button(w,'create').trigger('click');
    expect(w.get('[data-testid="snapshot-capture-policy"]').text()).toContain('Upgrade');
    expect(service.create).not.toHaveBeenCalled();
  });

  it("preserves the confirmed policy and blocks submission after the default changes", async () => {
    const current=overview(); current.policy.enabled=true; current.policies[0]!.enabled=true;
    service.overview.mockResolvedValue(current);
    const w=panel(); await flushPromises();
    await button(w,'create').trigger('click');
    current.policies=[{...current.policies[0]!,enabled:false,is_default:false},{...current.policies[0]!,id:'policy-2',name:'Upgrade',enabled:true,is_default:true}];
    current.default_policy_id='policy-2';
    service.overview.mockResolvedValue({...current});
    emitBackupEvent('snapshots'); await flushPromises();
    expect(w.get('form').text()).toContain('backupCenter.snapshots.capturePolicyChanged');
    expect(w.get('[data-testid="snapshot-capture-policy"]').text()).toContain('Daily');
    await w.get('form').trigger('submit'); await flushPromises();
    expect(service.create).not.toHaveBeenCalled();
  });

  it("allows online capture and reuses the capture key after an uncertain response", async () => {
    const current=overview(); current.policy.enabled=true; current.policies[0]!.enabled=true;
    service.overview.mockResolvedValue(current);
    service.create.mockRejectedValueOnce(new Error("network lost"));
    const w = panel();
    await flushPromises();
    await button(w, "create").trigger("click");
    expect(w.get('[data-testid="snapshot-capture-policy"]').text()).toContain("Daily");
    expect(w.find('input[type="checkbox"]').exists()).toBe(false);
    await w.get("form").trigger("submit");
    await flushPromises();
    await w.get("form").trigger("submit");
    await flushPromises();
    expect(service.create).toHaveBeenCalledTimes(2);
    expect(service.create).toHaveBeenCalledWith(expect.any(String), "policy-1");
    expect(service.create.mock.calls[0]![0]).toEqual(
      service.create.mock.calls[1]![0],
    );
  });
  it("shows a pushed backup retry without pausing business or resetting the draft", async () => {
    const current=overview();
    service.overview.mockResolvedValue(current);
    const w=panel();await flushPromises();
    await button(w,'create').trigger('click');
    current.snapshots=[{...snapshot,status:'capturing',phase:'retry_wait',attempt:2,retry_at:'2026-10-18T12:00:00Z',error:'source changed'}];
    service.overview.mockResolvedValue({...current});
    emitBackupEvent('snapshots');await flushPromises();
    expect(w.text()).toContain('backupCenter.snapshots.retryWaiting');
    expect(w.text()).toContain('backupCenter.snapshots.retryAt');
    expect(w.get('form').text()).toContain('backupCenter.snapshots.manualCaptureHint');
    expect(w.find('input[type="checkbox"]').exists()).toBe(false);
  });
  it("keeps a pushed failure when it arrives before the queued creation receipt", async () => {
    const w=panel();await flushPromises();
    let created!: (value:any)=>void;
    service.create.mockImplementation(()=>new Promise(resolve=>{created=resolve}));
    await button(w,'create').trigger('click');
    const submitting=w.get('form').trigger('submit');await flushPromises();
    const queued={...snapshot,id:'new-snapshot',status:'queued',phase:'queued'};
    const failed={...queued,status:'failed',phase:'failed',error:'程序产物采集失败（plugin/backend）'};
    service.overview.mockResolvedValue({...overview(),snapshots:[failed]});
    emitBackupEvent('snapshots');await flushPromises();
    expect(w.get('[data-testid="system-snapshot"]').text()).toContain(failed.error);
    let refreshed!: (value:any)=>void;
    service.overview.mockImplementation(()=>new Promise(resolve=>{refreshed=resolve}));
    created(queued);await flushPromises();
    expect(w.get('[data-testid="system-snapshot"]').text()).toContain(failed.error);
    expect(w.get('[data-testid="system-snapshot"]').text()).not.toContain('backupCenter.snapshots.states.queued');
    refreshed({...overview(),snapshots:[failed]});await submitting;await flushPromises();
    await button(w,'details').trigger('click');
    expect(w.text()).toContain('backupCenter.snapshots.states.failed');
    expect(w.text()).toContain(failed.error);
  });
  it("shows the effective snapshot directory and opens configuration help without changing the store", async () => {
    const current=overview();service.overview.mockResolvedValue(current);
    const w=panel();await flushPromises();
    expect(w.get('[data-testid="snapshot-storage-path"]').text()).toBe(current.directory);
    expect(w.get('[data-testid="snapshot-storage"]').text()).toContain('backupCenter.snapshots.storageSources.configuration');
    await button(w,'storageConfigure').trigger('click');
    expect(w.get('[data-testid="snapshot-storage-help"]').text()).toContain('system_snapshots:');
    expect(w.get('[data-testid="snapshot-storage-help"]').text()).toContain('POWERX_SNAPSHOT_DIRECTORY');
    expect(w.get('[data-testid="snapshot-storage-help"]').text()).toContain('snapshot_directory');
    expect(service.savePolicy).not.toHaveBeenCalled();expect(service.create).not.toHaveBeenCalled();
  });
  it("removes obsolete data-copy and live-switch actions from snapshot details", async () => {
    const w=panel();await flushPromises();await button(w,'details').trigger('click');
    expect(w.findAll('button').some(item=>item.text()==='backupCenter.recovery.dataOnly')).toBe(false);
    expect(w.findAll('button').some(item=>item.text()==='backupCenter.recovery.cutover')).toBe(false);
    expect(service.restore).not.toHaveBeenCalled();
  });
  it("blocks capture when a required source is unavailable", async () => {
    service.overview.mockResolvedValue({
      ...overview(),
      policies: [{...overview().policies[0]!,enabled:true}],
      ready: false,
      problems: ["program unavailable"],
    });
    const w = panel();
    await flushPromises();
    expect(button(w, "create").attributes("disabled")).toBeDefined();
    expect(w.text()).toContain("program unavailable");
    expect(button(w, "import").attributes("disabled")).toBeUndefined();
  });
  it("does not turn a prepared data copy into business verified success", async () => {
    service.overview.mockResolvedValue({
      ...overview(),
      restores: [
        {
          id: "r",
          status: "prepared",
          created_at: snapshot.created_at,
          directory: "/owned/restore",
          database: "copy",
          warnings: ["not application verified"],
        },
      ],
    });
    const w = panel();
    await flushPromises();
    expect(w.text()).toContain("backupCenter.snapshots.states.prepared");
    expect(w.text()).toContain("not application verified");
    expect(service.restore).not.toHaveBeenCalled();
  });
  it("refreshes active tasks only on pushed changes and unsubscribes on unmount", async () => {
    vi.useFakeTimers();
    service.overview.mockResolvedValue({
      ...overview(),
      snapshots: [{ ...snapshot, status: "capturing" }],
    });
    const w = panel();
    await flushPromises();
    await vi.advanceTimersByTimeAsync(30000);
    expect(service.overview).toHaveBeenCalledTimes(1);
    emitBackupEvent('snapshots'); await flushPromises();
    expect(service.overview).toHaveBeenCalledTimes(2);
    w.unmount();
    emitBackupEvent('snapshots');
    await vi.advanceTimersByTimeAsync(10000);
    expect(service.overview).toHaveBeenCalledTimes(2);
  });
});
