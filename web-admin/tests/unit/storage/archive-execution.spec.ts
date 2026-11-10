import { describe, expect, it } from 'vitest';
import { archiveExecutionNeedsReview, archiveExecutionState } from '~/utils/archive-execution';
const job=(extra:any={})=>({status:'running',remote_state:'pending',...extra}) as any;
describe('archive execution evidence',()=>{
 it('requires current executor proof for running records',()=>{
  expect(archiveExecutionState(job())).toBe('unconfirmed');
  expect(archiveExecutionState(job({execution_state:'future'}))).toBe('unconfirmed');
  expect(archiveExecutionState(job({execution_state:'active'}))).toBe('active');
 });
 it('keeps interruption attention while explicitly retrying',()=>{
  expect(archiveExecutionState(job({interruption_pending:true}))).toBe('interrupted');
  expect(archiveExecutionState(job({interruption_pending:true,execution_state:'active'}))).toBe('active');
  expect(archiveExecutionNeedsReview(job({status:'success',remote_state:'failed',interruption_pending:true}))).toBe(true);
 });
 it('does not turn imported historical execution metadata into current ownership or a current gate',()=>{
  for(const history of [{imported:true},{recovered:true}])expect(archiveExecutionState(job({...history,interruption_pending:true,execution_state:'active'}))).toBe('recorded');
  expect(archiveExecutionNeedsReview(job({status:'success',remote_state:'success',execution_state:'recorded'}))).toBe(false);
 });
});
