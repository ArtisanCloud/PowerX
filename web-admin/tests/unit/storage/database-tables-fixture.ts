import type { DatabaseTableInventory, SystemTableObservation } from '~/composables/api/services/storageDatabaseTablesService';
export function tableInventory(): DatabaseTableInventory {
  const row: SystemTableObservation = { id: 'capability_registry_invocation_traces', schema: 'public', category: 'observability', time_column: 'created_at', cleanup_mechanism: 'invocation_policy', protection: 'terminal_and_unreferenced', state: 'observed', heap_bytes: 1024 ** 3, indexes_bytes: 1024, toast_bytes: 4096, total_bytes: 1024 ** 3 + 5120, estimated_rows: 12345, time_state: 'observed', oldest_at: '2026-10-01T00:00:00Z', newest_at: '2026-11-04T00:00:00Z' };
  return { observed_at: '2026-11-04T00:00:00Z', expires_at: '2026-11-04T00:02:00Z', identity: { instance_id: 'fixture-install', environment: 'dev' }, scope: 'current_database_registered_tables_including_soft_deleted', monitoring: tableMonitoring(), invocation_policy: null, items: [row,
    { ...row, id: 'skills_execution_traces', cleanup_mechanism: 'review_required', state: 'not_present', heap_bytes: null, indexes_bytes: null, toast_bytes: null, total_bytes: null, estimated_rows: null, time_state: 'unavailable', oldest_at: null, newest_at: null, problem: 'table_not_present' },
    { ...row, id: 'iam_authorization_segments', category: 'protected', cleanup_mechanism: 'protected', protection: 'excluded_from_log_cleanup', heap_bytes: 8192, indexes_bytes: 0, toast_bytes: 0, total_bytes: 8192, time_state: 'empty', oldest_at: null, newest_at: null, estimated_rows: 0 },
  ] };
}

export function tableMonitoring(): import('~/composables/api/services/storageDatabaseTablesService').TableMonitoringStatus {
 return {configuration:{authority:'database',ready:true,state:'applied',desired_revision:1,applied_revision:1,snapshot_revision:1,desired_policy:{enabled:false,warning_bytes:4*2**30,growth_bytes:256*2**20,growth_window_minutes:15,stale_after_minutes:45,failure_limit:3},legacy_revision:0,legacy_digest:'a'.repeat(64),legacy_enabled:false},state:'disabled',state_configured:true,sampling_configured:true,worker_enabled:true,sampled_at:null,tables:[],alerts:[],cleanup:null,backlog:null,policy:{revision:1,owner_instance:'fixture-install',enabled:false,warning_bytes:4*2**30,growth_bytes:256*2**20,growth_window_minutes:15,stale_after_minutes:45,failure_limit:3,updated_at:'0001-01-01T00:00:00Z',actor:0}};
}
