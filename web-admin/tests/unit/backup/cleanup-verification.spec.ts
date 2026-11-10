import { describe, expect, it, vi } from 'vitest';
import { consumeCleanupVerification, decodeCleanupVerification, useBackupCleanupReviewService } from '~/composables/api/services/backupCleanupReviewService';
const transport = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('~/composables/api', () => ({ useApiClient: () => transport }));
const token='7a3bf1aa-2735-458a-bd5b-b9a4e102c0bd';
const running=()=>({artifact_id:'1',token,state:'running',bytes_read:0,total_bytes:4,dump_matched:false,content_verified:false,read_only:true,cleanup_unchanged:true,observed_at:'2026-11-09T01:00:00Z'});
const complete=()=>({...running(),state:'verified',bytes_read:4,dump_matched:true,content_verified:true});
const frame=(name:string,value:any)=>`event: ${name}\ndata: ${JSON.stringify(value)}\n\n`;
const stream=(raw:string,chunk=17)=>new ReadableStream<Uint8Array>({start(c){const data=new TextEncoder().encode(raw);for(let i=0;i<data.length;i+=chunk)c.enqueue(data.slice(i,i+chunk));c.close()}});
describe('one-shot backup integrity SSE',()=>{
 it('requires terminal exact-token proof and validates all safety flags',()=>{
  expect(decodeCleanupVerification(complete()).content_verified).toBe(true);
  for(const change of [{state:'checksum_mismatch'},{read_only:false},{cleanup_unchanged:false},{dump_matched:false},{bytes_read:3},{total_bytes:0}])expect(()=>decodeCleanupVerification({...complete(),...change})).toThrow();
 });
 it('reads split frames, reports progress and returns only a complete result',async()=>{
  const progress=vi.fn();const result=await consumeCleanupVerification(stream(frame('progress',running())+frame('complete',complete())), '1',token,new AbortController().signal,progress);
  expect(progress).toHaveBeenCalledTimes(1);expect(result.state).toBe('verified');
 });
 it('rejects EOF, reconnect instructions and mismatched identity without retrying',async()=>{
  for(const raw of [frame('progress',running()),frame('reconnect',{}),frame('complete',{...complete(),artifact_id:'2'}),frame('complete',{...complete(),token:'ac5369d5-616f-4532-a089-11fcf34d9ab4'}),frame('complete',running())])await expect(consumeCleanupVerification(stream(raw),'1',token,new AbortController().signal,()=>{})).rejects.toThrow();
 });
 it('cancels an in-flight read without accepting a late success',async()=>{
  let closed=false;const controller=new AbortController();const source=new ReadableStream<Uint8Array>({start(c){c.enqueue(new TextEncoder().encode(frame('progress',running())))},cancel(){closed=true}});
  const progress=vi.fn(()=>controller.abort());await expect(consumeCleanupVerification(source,'1',token,controller.signal,progress)).rejects.toThrow();expect(closed).toBe(true);
 });
 it('opens one explicit stream with retries and global loading disabled',async()=>{
  transport.get.mockResolvedValue(stream(frame('complete',complete())));const controller=new AbortController();await useBackupCleanupReviewService().verify('1',token,controller.signal,()=>{});
  expect(transport.get).toHaveBeenCalledTimes(1);expect(transport.get).toHaveBeenCalledWith(expect.stringContaining('/cleanup-reviews/1/verification?expected_token='),expect.objectContaining({retry:0,timeout:0,responseType:'stream',useGlobalLoading:false,useGlobalError:false,signal:controller.signal}));
 });
});
