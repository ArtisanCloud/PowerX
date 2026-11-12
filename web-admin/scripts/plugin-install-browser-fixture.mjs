import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {randomUUID} from 'node:crypto';
const webPort=39581,apiPort=39582;
const dir=resolve('../tmp/plugin-install-tasks-20261112/browser');mkdirSync(dir,{recursive:true});
const entry=resolve(process.env.POWERX_PLUGIN_INSTALL_UI_ENTRY || '../tmp/plugin-install-tasks-20261112/nuxt-product-output/server/index.mjs');
const tenant='00000000-0000-4000-8000-000000000001';let task;const streams=new Set();const requests=[];const timers=new Set();
const json=(res,data,code=200)=>{res.writeHead(code,{'content-type':'application/json'});res.end(JSON.stringify({code,message:'success',data}));};
const notify=()=>{for(const res of streams)res.write(`event: status\ndata: ${JSON.stringify(task)}\n\n`);};
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://127.0.0.1').pathname;
 if(path==='/__fixture/reset'){for(const stream of streams)stream.end();streams.clear();task=undefined;requests.length=0;json(res,{});return;}
 if(path==='/__fixture/status'){json(res,{task,requests,subscribers:streams.size});return;}
 if(path==='/__fixture/complete'){task={...task,status:'succeeded',phase:'completed',revision:task.revision+1};notify();json(res,{});return;}
 requests.push({method:req.method,path,at:Date.now()});
 if(path.endsWith('/installations')&&req.method==='POST'){
   let body='';for await(const chunk of req)body+=chunk;const inTask=JSON.parse(body);
   task={task_id:randomUUID(),request_id:inTask.request_id,plugin_id:inTask.plugin_id,version:inTask.version,source_kind:inTask.source_kind,status:'awaiting_upload',phase:'upload',revision:1,created_at:new Date().toISOString(),updated_at:new Date().toISOString()};json(res,task);return;
 }
 if(path.endsWith('/installations')){json(res,{items:task?[task]:[]});return;}
 if(/\/installations\/[^/]+\/package$/.test(path)){
   for await(const _chunk of req){};task={...task,status:'queued',phase:'queued',revision:2};json(res,task);
   const timer=setTimeout(()=>{timers.delete(timer);task={...task,status:'running',phase:'activating',revision:3};notify();},100);timers.add(timer);return;
 }
 if(/\/installations\/[^/]+\/events$/.test(path)){
   res.writeHead(200,{'content-type':'text/event-stream','cache-control':'no-store','x-accel-buffering':'no'});res.write(`event: status\ndata: ${JSON.stringify(task)}\n\n`);streams.add(res);res.once('close',()=>streams.delete(res));return;
 }
 if(/\/installations\/[^/]+$/.test(path)){json(res,task);return;}
 if(path.endsWith('/me/context')){json(res,{is_root:true,current_tenant_uuid:tenant,current_member_id:1,user:{id:1,display_name:'Install fixture',status:1,is_root:true},members:[{tenant_uuid:tenant,tenant_name:'Fixture',member_id:1,is_admin:true}]});return;}
 if(path.endsWith('/setup/status')){json(res,{configured:true,requires_login:true,version:'test'});return;}
 if(path.endsWith('/menus')){json(res,{menus:[{id:'plugins',title:'本地插件',path:'/plugins/local',visible:true,origin:'system'}]});return;}
 if(path.endsWith('/marketplace/plugins')){json(res,{items:[]});return;}
 if(path.endsWith('/tenant-instances')){json(res,[]);return;}
 if(/\/admin\/plugins\/?$/.test(path)){json(res,{plugins:[]});return;}
 json(res,{items:[],unread_count:0});
});
await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(apiPort,'127.0.0.1',resolve);});
const child=spawn(process.execPath,[entry],{env:{...process.env,PORT:String(webPort),HOST:'127.0.0.1',POWERX_BACKEND:`http://127.0.0.1:${apiPort}`},stdio:'inherit'});
writeFileSync(resolve(dir,'owned-processes.json'),JSON.stringify({owner:'Codex plugin install browser fixture',cwd:process.cwd(),processes:[{pid:process.pid,command:'node scripts/plugin-install-browser-fixture.mjs',port:apiPort},{pid:child.pid,command:`node ${entry}`,port:webPort}]},null,2));
let closing=false;
async function close(){if(closing)return;closing=true;for(const timer of timers)clearTimeout(timer);for(const res of streams)res.end();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));child.kill('SIGTERM');await new Promise(resolve=>{child.once('exit',resolve);setTimeout(()=>{child.kill('SIGKILL');resolve();},4000).unref();});writeFileSync(resolve(dir,'requests.json'),JSON.stringify(requests,null,2));process.exit(0);}
process.on('SIGTERM',close);process.on('SIGINT',close);child.once('exit',()=>{if(!closing)void close();});
