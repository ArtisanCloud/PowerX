import { useApiClient } from '~/composables/api';
import { decodeCleanupVerification, type BackupCleanupVerification } from './backupCleanupReviewService';
export interface BackupRetentionInput { request_id:string; expected_token:string; review_version:string; reason:string; confirm:true }
export interface BackupRetentionReceipt { request_id:string; artifact_id:string; token:string; review_version:string; reason:string; state:'checking'|'retained'|'rejected'; problem?:string; actor_id:string; created_at:string; completed_at:string|null; can_resume:boolean; protected:boolean; cleanup_lock_preserved:true; record_source?:'database'|'maintenance'; recovery_blocked?:boolean }
const uuid=(v:any)=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v)&&!/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(v);
const id=(v:any)=>typeof v==='string'&&/^[1-9][0-9]{0,19}$/.test(v);
const date=(v:any)=>typeof v==='string'&&/^\d{4}-\d{2}-\d{2}T/.test(v)&&Number.isFinite(Date.parse(v));
const invalid=():never=>{throw new Error('BACKUP_RETAIN_RESPONSE_INVALID')};
export function decodeRetentionReceipt(raw:any):BackupRetentionReceipt{
 const v=raw&&typeof raw==='object'&&'data' in raw?raw.data:raw;
 if(!v||!uuid(v.request_id)||!id(v.artifact_id)||!uuid(v.token)||typeof v.review_version!=='string'||!/^[a-f0-9]{64}$/.test(v.review_version)||typeof v.reason!=='string'||[...v.reason].length>256||!['checking','retained','rejected'].includes(v.state)||!id(v.actor_id)||!date(v.created_at)||v.state==='checking'&&v.completed_at!==null||v.state!=='checking'&&!date(v.completed_at)||typeof v.can_resume!=='boolean'||v.can_resume&&v.state!=='checking'||v.protected!==(v.state==='retained')||v.cleanup_lock_preserved!==true||v.record_source!==undefined&&!['database','maintenance'].includes(v.record_source)||v.recovery_blocked!==undefined&&typeof v.recovery_blocked!=='boolean'||v.recovery_blocked===true&&v.can_resume||v.problem!==undefined&&(typeof v.problem!=='string'||!/^[A-Z][A-Z0-9_]*$/.test(v.problem)))return invalid();return v;
}
export async function consumeRetentionStream(stream:ReadableStream<Uint8Array>,artifact:string,input:BackupRetentionInput,signal:AbortSignal,receive:(value:{receipt?:BackupRetentionReceipt;progress?:BackupCleanupVerification})=>void):Promise<BackupRetentionReceipt>{
 const reader=stream.getReader(),decoder=new TextDecoder();let buffer='';const abort=()=>{void reader.cancel().catch(()=>{})};signal.addEventListener('abort',abort,{once:true});
 try{for(;;){if(signal.aborted)throw new Error('BACKUP_RETAIN_RESULT_UNKNOWN');const{value,done}=await reader.read();if(done||signal.aborted)throw new Error('BACKUP_RETAIN_RESULT_UNKNOWN');buffer=(buffer+decoder.decode(value,{stream:true})).replace(/\r\n/g,'\n');if(buffer.length>1048576)return invalid();let end:number;
  while((end=buffer.indexOf('\n\n'))>=0){const frame=buffer.slice(0,end);buffer=buffer.slice(end+2);let event='';const data:string[]=[];for(const line of frame.split('\n')){if(line.startsWith('event:'))event=line.slice(6).trim();if(line.startsWith('data:'))data.push(line.slice(5).trimStart())};if(!data.length)continue;const v=JSON.parse(data.join('\n'));
   if(event==='progress'){const progress=decodeCleanupVerification(v);if(progress.artifact_id!==artifact||progress.token!==input.expected_token||progress.state!=='running')return invalid();receive({progress});continue}
   if(event!=='accepted'&&event!=='complete')throw new Error('BACKUP_RETAIN_RESULT_UNKNOWN');const receipt=decodeRetentionReceipt(v);if(receipt.artifact_id!==artifact||receipt.request_id!==input.request_id||receipt.token!==input.expected_token||receipt.review_version!==input.review_version||receipt.reason!==input.reason)return invalid();
   if(event==='complete'){if(receipt.state==='checking'&&!receipt.recovery_blocked)return invalid();return receipt};if(receipt.state!=='checking')return invalid();receive({receipt});
  }
 }}finally{signal.removeEventListener('abort',abort);await reader.cancel().catch(()=>{});reader.releaseLock()}
}
export function useBackupRetentionService(){
 const api=useApiClient();const config=(signal?:AbortSignal)=>({signal,retry:0,useGlobalLoading:false,useGlobalError:false});
 return{
  async list(artifact:string,token:string,signal?:AbortSignal){if(!id(artifact)||!uuid(token))return invalid();const raw:any=await api.get(`/admin/ops/backup/cleanup-reviews/${artifact}/retentions?expected_token=${token}`,config(signal));const v=raw?.data??raw;if(!Array.isArray(v?.items)||v.items.length>20)return invalid();const rows=v.items.map(decodeRetentionReceipt) as BackupRetentionReceipt[];if(rows.some(r=>r.artifact_id!==artifact||r.token!==token))return invalid();return rows},
  async request(key:string,signal?:AbortSignal){if(!uuid(key))return invalid();const value=decodeRetentionReceipt(await api.get(`/admin/ops/backup/cleanup-retentions/${key}`,config(signal)));if(value.request_id!==key)return invalid();return value},
  async retain(artifact:string,input:BackupRetentionInput,signal:AbortSignal,receive:(value:{receipt?:BackupRetentionReceipt;progress?:BackupCleanupVerification})=>void){if(!id(artifact)||!uuid(input.request_id)||!uuid(input.expected_token)||!/^[a-f0-9]{64}$/.test(input.review_version)||input.confirm!==true)return invalid();const stream=await api.post<ReadableStream<Uint8Array>>(`/admin/ops/backup/cleanup-reviews/${artifact}/retentions`,input,{...config(signal),timeout:0,responseType:'stream',headers:{Accept:'text/event-stream'}});return consumeRetentionStream(stream,artifact,input,signal,receive)}
 }
}
