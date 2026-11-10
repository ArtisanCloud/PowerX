import {beforeEach,expect,it,vi} from 'vitest';
import {tableMonitoring} from './database-tables-fixture';
const transport=vi.hoisted(()=>({get:vi.fn(),put:vi.fn(),post:vi.fn()}));
vi.mock('~/composables/api',()=>({useApiClient:()=>transport}));
import {useStorageDatabaseTablesService,decodeMonitoringConfiguration,monitoringSettings} from '~/composables/api/services/storageDatabaseTablesService';
const key='00000000-0000-4000-8000-000000000001';
beforeEach(()=>vi.clearAllMocks());
it('sends only editable settings with a durable request ID and original CAS version, without HTTP retries',async()=>{
 const p=tableMonitoring().policy,configuration={...tableMonitoring().configuration!,state:'pending',desired_revision:2};
 transport.put.mockResolvedValue({data:{request_id:key,operation_revision:2,operation_policy:p,configuration}});
 await useStorageDatabaseTablesService().saveMonitoring(new AbortController().signal,p,1,key);
 expect(transport.put.mock.calls[0]![1]).toEqual({request_id:key,expected_revision:1,confirm:true,policy:monitoringSettings(p)});
 expect(transport.put.mock.calls[0]![2]).toMatchObject({retry:0,useGlobalLoading:false,timeout:15000});
 expect(transport.put.mock.calls[0]![1].policy).not.toHaveProperty('actor');expect(transport.put.mock.calls[0]![1].policy).not.toHaveProperty('revision');
});
it('reconciles an unknown operation through a GET of the same request, never through another write',async()=>{
 const p=tableMonitoring().policy;transport.get.mockResolvedValue({request_id:key,operation_revision:1,operation_policy:p,configuration:tableMonitoring().configuration});
 await useStorageDatabaseTablesService().monitoringRequest(new AbortController().signal,key);
 expect(transport.get.mock.calls[0]![0]).toBe('/admin/ops/storage/database-tables/monitoring/requests/'+key);expect(transport.post).not.toHaveBeenCalled();expect(transport.put).not.toHaveBeenCalled();
});
it('rejects a claimed applied state with mismatching database and local versions',()=>{
 expect(()=>decodeMonitoringConfiguration({...tableMonitoring().configuration,desired_revision:2,applied_revision:1})).toThrow('STORAGE_TABLE_CONFIGURATION_RESPONSE_INVALID');
});
