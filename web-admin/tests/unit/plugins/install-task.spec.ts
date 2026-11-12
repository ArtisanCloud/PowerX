import {expect,it} from 'vitest';
import {acceptInstallTask,installTaskTerminal,type PluginInstallTask} from '~/utils/plugin-install-task';
const task={task_id:'a',request_id:'req',plugin_id:'plugin',version:'1',source_kind:'local',status:'running',phase:'migrating',revision:4,created_at:'',updated_at:''} as PluginInstallTask;
it('rejects other tasks and stale revisions',()=>{expect(acceptInstallTask(task,{...task,task_id:'b',revision:9})).toBe(false);expect(acceptInstallTask(task,{...task,revision:3})).toBe(false);expect(acceptInstallTask(task,{...task,revision:5})).toBe(true);});
it('terminal receipts cannot regress or be rewritten by late events',()=>{const done={...task,status:'succeeded' as const};expect(installTaskTerminal(done)).toBe(true);expect(acceptInstallTask(done,{...task,revision:10})).toBe(false);expect(acceptInstallTask(done,{...done,revision:5})).toBe(false);expect(acceptInstallTask(done,{...done})).toBe(true);});
