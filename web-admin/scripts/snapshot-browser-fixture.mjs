import { randomUUID } from "node:crypto";
import { createServer } from "node:http";
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { delimiter, resolve } from "node:path";

const webPort = Number(process.env.POWERX_SNAPSHOT_BROWSER_PORT || 3477);
const backendPort = Number(
  process.env.POWERX_SNAPSHOT_BROWSER_BACKEND_PORT || 3478,
);
if (
  ![webPort, backendPort].every(
    (port) => Number.isInteger(port) && port > 1024 && port < 65536,
  ) ||
  webPort === backendPort
) {
  throw new Error(
    "Snapshot browser tests require two distinct unprivileged ports",
  );
}
const entry = resolve(process.env.POWERX_SNAPSHOT_WEB_ENTRY || ".output/server/index.mjs");
if (!existsSync(entry))
  throw new Error("Run npm run build before the snapshot browser tests");

// Browser-only API fixtures. Encryption and native PostgreSQL restore are tested in Go.
const requests = [];
const tenantUuid = "snapshot-browser-fixture";
const context = {
  is_root: true,
  current_tenant_uuid: tenantUuid,
  current_member_id: 1,
  user: {
    id: 1,
    email: "snapshot@example.invalid",
    display_name: "Snapshot fixture",
    status: 1,
    is_root: true,
  },
  members: [
    {
      tenant_uuid: tenantUuid,
      tenant_name: "Snapshot fixture",
      member_id: 1,
      is_admin: true,
    },
  ],
};
const base = "/api/v1/admin/ops/backup/system-snapshots";
const snapshot = {
  policy_id: "5809c3a0-1280-422d-81fe-fbbb60e55d46",
  policy_name: "每日快照",
  id: "9282f978-33e4-4d7e-a36f-8e8f39536251",
  status: "ready",
  phase: "complete",
  created_at: "2026-10-15T01:00:00Z",
  captured_at: "2026-10-15T01:00:01Z",
  updated_at: "2026-10-15T01:01:00Z",
  instance: "browser-fixture",
  version: "fixture-v1",
  trigger: "manual",
  protected: false,
  imported: false,
  business_verified: false,
  components: ["database", "media", "configuration", "programs"].map(
    (kind) => ({ kind, status: "verified", file_count: 1, size_bytes: 1024 }),
  ),
};
let overview;
let databaseExports = [], mediaExports = [], emptyPolicies = false, deleteFailure = false;
 let recovery;
 const subscribers = new Set();
 const jobSubscribers = new Map();
 const pendingUpdates = new Set();
 let coreStopped = false;
 function later(action, delay) { const timer = setTimeout(() => { pendingUpdates.delete(timer); action(); }, delay); pendingUpdates.add(timer); }
 function notify(...resources) { for (const response of subscribers) response.write(`event: invalidate\ndata: ${JSON.stringify({resources})}\n\n`); }
 function progress(job) { for (const response of jobSubscribers.get(job.id) || []) response.write(`event: progress\ndata: ${JSON.stringify({recovery:job})}\n\n`); }
 function eventStream(request,response,list,event,data) { response.writeHead(200,{"content-type":"text/event-stream","cache-control":"no-store","x-accel-buffering":"no"});response.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);list.add(response);response.once("close",()=>list.delete(response)); }
 const progressToken="1999999999."+"a".repeat(64);
function reset() {
  for (const timer of pendingUpdates) clearTimeout(timer); pendingUpdates.clear();
  for (const response of subscribers) response.end(); subscribers.clear();
  for (const list of jobSubscribers.values()) for (const response of list) response.end(); jobSubscribers.clear();
  coreStopped = false;
  databaseExports=[];mediaExports=[];emptyPolicies=false;deleteFailure=false;
  overview = {
    directory: "/var/lib/powerx/snapshot fixture/store",
    directory_source: "configuration",
    ready: true,
    problems: [],
    instance: "browser-fixture",
    policy: {
      enabled: false,
      interval_minutes: 1440,
      retention_days: 7,
      retention_count: 7,
    },
    snapshots: [structuredClone(snapshot)],
    restores: [],
    cutover_available: false,
  };
  overview.default_policy_id = '5809c3a0-1280-422d-81fe-fbbb60e55d46';
  overview.policies = [{...overview.policy,id:overview.default_policy_id,name:'每日快照',is_default:true}];
  recovery={configured:false,online:false,generation:1,targets:[],jobs:[],progress_tokens:{}};
 requests.length = 0;
}
reset();
function json(response, status, data) {
  response.writeHead(status, {
    "content-type": "application/json",
    "cache-control": "no-store",
  });
  response.end(JSON.stringify(data));
}
const backend = createServer(async (request, response) => {
  const url = new URL(request.url || "/", `http://127.0.0.1:${backendPort}`);
  if (url.pathname === "/api/v1/__snapshot_browser/state") {
    if (request.method === "DELETE") reset();
    json(response, 200, { requests, overview, eventConnections: subscribers.size, progressConnections: [...jobSubscribers.values()].reduce((count,list)=>count+list.size,0) });
    return;
  }
  if(url.pathname === "/api/v1/__snapshot_browser/managed") {recovery={configured:true,online:true,generation:1,active_environment_id:"source",targets:[{id:"formal",label:"本站正式候选",environment:"dev",mode:"formal",allow_overwrite:true,allow_online_data:true}],jobs:[],progress_tokens:{}};overview.snapshots[0].recovery_supported=true;json(response,200,{ok:true});return;}
  if(url.pathname==='/api/v1/__snapshot_browser/empty-snapshots'){overview.snapshots=[];notify('snapshots');json(response,200,{ok:true});return;}
  if(url.pathname==='/api/v1/__snapshot_browser/empty-policies'){emptyPolicies=true;json(response,200,{ok:true});return;}
  if(url.pathname === "/api/v1/__snapshot_browser/push") { let raw='';for await(const chunk of request)raw+=chunk;const changes=JSON.parse(raw);Object.assign(overview.snapshots[0],changes);notify('snapshots');json(response,200,{ok:true});return; }
  if(url.pathname==='/api/v1/__snapshot_browser/delete-failure') { let raw='';for await(const chunk of request)raw+=chunk;deleteFailure=JSON.parse(raw).enabled;json(response,200,{ok:true});return; }
  if(url.pathname==='/api/v1/__snapshot_browser/recovery-history') {
    let raw='';for await(const chunk of request)raw+=chunk;const input=JSON.parse(raw);
    recovery.jobs=input.statuses.map((status,index)=>({id:randomUUID(),kind:'restore',snapshot_id:snapshot.id,status,phase:status,created_at:`2026-10-15T00:00:0${index}Z`,error:status==='failed'?'历史预检查失败':undefined,checks:[]}));
    recovery.progress_tokens={};
    if(input.extraSnapshots)overview.snapshots=[{...structuredClone(snapshot),protected:true},{...structuredClone(snapshot),id:'45818353-20cd-4398-b739-f3548a5ba003'},{...structuredClone(snapshot),id:'332724fc-203f-4992-9546-02de2b9ad38f',status:'failed',phase:'failed',error:'历史快照采集失败',components:[]}];
    notify('snapshots','recovery');json(response,200,{ok:true});return;
  }
  const hasBearer = String(request.headers.authorization || "").startsWith(
    "Bearer ",
  );
  requests.push({
    method: request.method,
    path: url.pathname,
    view: url.searchParams.get("view"),
    hasBearer,
    idempotencyKey: request.headers["idempotency-key"] || "",
  });
  if(url.pathname==='/api/v1/admin/ops/backup/events') {
    if(!hasBearer){json(response,401,{});return;}
    if(coreStopped){json(response,503,{message:'Core is stopped'});return;}
    eventStream(request,response,subscribers,'invalidate',{resources:['snapshots','recovery','archives','restore_copies']});return;
  }
  if(url.pathname==='/api/v1/admin/ops/backup/jobs/run'&&request.method==='POST'){
    let raw='';for await(const part of request)raw+=part;const input=JSON.parse(raw);requests.at(-1).body=input;
    if(!input.one_time||input.policy_id){json(response,400,{message:'Expected policy-free backup'});return;}
    const job={id:9001,policy_id:0,status:'success',trigger_type:'manual',operator:'root',storage_uri:'file:///fixture/on-demand-database.dump',size_bytes:2048,started_at:new Date().toISOString()};databaseExports.unshift(job);json(response,200,{data:{job}});return;
  }
  if(/^\/api\/v1\/admin\/ops\/backup\/jobs\/\d+\/download$/.test(url.pathname)){response.writeHead(200,{'content-type':'application/postgresql-custom','content-disposition':'attachment; filename="database.dump"'});response.end('PGDMP-browser-fixture');return;}
  if(url.pathname==='/api/v1/admin/ops/backup/archives/jobs'&&request.method==='POST'){
    let raw='';for await(const part of request)raw+=part;const input=JSON.parse(raw);requests.at(-1).body=input;
    const job={id:'40b5744c-7e35-4c70-a31a-0123b9943419',policy_id:'',one_time:true,source_id:input.source_id,kind:'media',status:'success',phase:'completed',created_at:new Date().toISOString(),file_count:2,size_bytes:1024,new_bytes:1024,remote_state:'not_requested',protected:false,warnings:[]};mediaExports.unshift(job);json(response,202,{data:{job}});return;
  }
  if(url.pathname.startsWith('/api/v1/admin/ops/backup/archives/jobs/')&&url.pathname.endsWith('/download')){response.writeHead(200,{'content-type':'application/gzip'});response.end('media-browser-fixture');return;}
  if (url.pathname === "/api/v1/admin/ops/backup/jobs") {
    const jobs = [...databaseExports,
      { id: 801, policy_id: 1, status: "success", trigger_type: "manual", operator: "root", storage_uri: "file:///fixture/legacy-available.dump", size_bytes: 2048, started_at: "2026-10-15T01:00:00Z" },
      { id: 799, policy_id: 1, status: "success", trigger_type: "scheduled", operator: "system", started_at: "2026-10-14T01:00:00Z" },
      { id: 798, policy_id: 1, status: "failed", trigger_type: "scheduled", operator: "system", error_message: "fixture backup failure", started_at: "2026-10-13T01:00:00Z" },
    ];
    const items = url.searchParams.get("view") === "available" ? jobs.filter(job => job.status === "success" && job.storage_uri) : jobs;
    json(response, 200, { data: { items, pagination: { total: items.length, page: 1, page_size: 20 } } });
    return;
  }
  if (url.pathname === "/api/v1/admin/ops/backup/policies") {
    const items = (emptyPolicies ? [] : [1, 2]).map(id => ({id, name: `验证策略 ${id}`, enabled:false, schedule:'1h', interval_hours:1, retention_count:168, retention_days:7, retention_mode:'age_and_count', drill_enabled:false, drill_interval_days:7, timezone:'Asia/Shanghai'}));
    json(response,200,{data:{items,pagination:{total:items.length,page:1,page_size:200}}}); return;
  }
  if (url.pathname === "/api/v1/admin/monitor/backup/overview") {
    json(response,200,{data:{overview:{policies_enabled:0},runtime:{ready:true,problems:[],source_host:'127.0.0.1',source_port:5432,source_database:'fixture_database',artifact_directory:'/fixture/backups',path_template:'job_<id>.dump',format:'PostgreSQL custom dump'}}}); return;
  }
  if (url.pathname === "/api/v1/admin/ops/backup/archives") {
    json(response,200,{data:{overview:{instance:'fixture',directory:'/fixture/archives',sources:[{id:'media',kind:'media',name:'媒体来源',directory:'/fixture/media',ready:true}],credentials:[],policies:[],targets:[],jobs:mediaExports,restores:[],pending_transfers:0}}}); return;
  }
  if(url.pathname===base+"/recovery"&&request.method==="GET") {if(coreStopped){json(response,503,{error:"Core is stopped during fixture maintenance"});return;}json(response,200,{data:recovery});return;}
  if(url.pathname===base+"/"+snapshot.id+"/recovery"&&request.method==="POST") {
    let body="";for await(const part of request)body+=part;const input=JSON.parse(body);
    if(input.target_id || input.restore_mode || input.generation!==recovery.generation){json(response,400,{error:"restore contract mismatch"});return;}
    const job={id:randomUUID(),kind:"restore",restore_mode:"replace_instance",snapshot_id:snapshot.id,target_id:"current",status:"queued",phase:"queued",business_verified:false,writes_may_have_resumed:false,created_at:new Date().toISOString(),updated_at:new Date().toISOString(),checks:[{name:"database_rows",passed:true,detail:"全部业务记录与恢复点一致"}]};
    recovery.jobs.unshift(job);recovery.progress_tokens[job.id]=progressToken;json(response,202,{data:{recovery:job,progress_token:progressToken}});
    later(()=>{job.status='running';job.phase='maintenance';coreStopped=true;recovery.maintenance=true;progress(job);for(const response of subscribers)response.end();subscribers.clear();},150);
    later(()=>{job.phase='instance_replaced';job.rollback_snapshot_id=snapshot.id;job.updated_at=new Date().toISOString();progress(job);},400);
    later(()=>{job.status='complete';job.phase='complete';job.business_verified=true;job.writes_may_have_resumed=true;job.updated_at=new Date().toISOString();recovery.maintenance=false;coreStopped=false;recovery.generation++;progress(job);},800);return;
  }
  if(url.pathname.startsWith(base+"/recovery/")&&request.method==='POST'){json(response,410,{error:"environment switching removed"});return;}
  if(url.pathname.startsWith("/__powerx_recovery/jobs/")) {
    const id=url.pathname.split("/")[3];const job=recovery.jobs.find(job=>job.id===id);
    if(!job||request.headers.authorization!=="Bearer "+progressToken){json(response,401,{error:"ticket required"});return;}
    if(url.pathname.endsWith('/events')){if(!jobSubscribers.has(id))jobSubscribers.set(id,new Set());eventStream(request,response,jobSubscribers.get(id),'progress',{recovery:job});return;}
    json(response,200,{recovery:job});return;
  }
  if (url.pathname === base && request.method === "GET") {
    json(response, 200, { data: { overview } });
    return;
  }
  const deletion = url.pathname.match(new RegExp('^'+base+'/([0-9a-f-]{36})$'));
  if (deletion && request.method === 'DELETE') {
    const id = deletion[1], point = overview.snapshots.find(row => row.id === id);
    requests.at(-1).confirmation = request.headers['x-snapshot-confirmation'];
    requests.at(-1).deleteProtected = request.headers['x-snapshot-delete-protected'];
    if (!point) { json(response,404,{message:'Snapshot not found'});return; }
    if (request.headers['x-snapshot-confirmation'] !== id) { json(response,400,{message:'Snapshot confirmation required'});return; }
    if (['queued','capturing','verifying'].includes(point.status)) { json(response,409,{message:'Snapshot is active'});return; }
    if (recovery.jobs.some(job=>['queued','running','attention_required'].includes(job.status))) { json(response,409,{message:'Recovery is active or needs attention'});return; }
    if (point.protected && request.headers['x-snapshot-delete-protected'] !== 'true') { json(response,409,{message:'Protected snapshot confirmation required'});return; }
    if (deleteFailure) { point.status='delete_failed';point.error='快照文件清理未完成，请重试删除';notify('snapshots');json(response,500,{message:point.error});return; }
    overview.snapshots=overview.snapshots.filter(row=>row.id!==id);notify('snapshots');
    json(response,200,{data:{deletion:{id,record_deleted:true,files_deleted:true}}});return;
  }
  if (url.pathname === base && request.method === "POST") {
    let raw='';for await(const part of request)raw+=part;const input=JSON.parse(raw);requests.at(-1).body=input;
    const policy=overview.policies.find(row=>row.id===input.policy_id&&row.is_default);
    if(!policy){json(response,409,{message:'Selected policy is not the system default'});return;}
    overview.snapshots.unshift({
      ...structuredClone(snapshot),
      policy_id:policy.id,
      policy_name:policy.name,
      id: "79bfa1ee-997f-4d5b-9a56-338cfe08b3cb",
    });
    json(response, 202, { data: { snapshot: overview.snapshots[0] } });
    return;
  }
  const policyPath = url.pathname.match(new RegExp('^'+base+'/policies/([^/]+)(/status)?$'));
  if (url.pathname === base+'/policies' || policyPath) {
    if(request.method==='GET'){json(response,200,{data:{policies:overview.policies,default_policy_id:overview.default_policy_id}});return;}
    let raw='';for await(const chunk of request)raw+=chunk;const input=JSON.parse(raw);requests.at(-1).body=input;
    let row;
    if(request.method==='POST'){
      row={...input,id:randomUUID(),is_default:false};overview.policies.push(row);
      if(!overview.default_policy_id)overview.default_policy_id=row.id;
    }else{
      row=overview.policies.find(p=>p.id===policyPath?.[1]);
      if(!row){json(response,404,{message:'Policy not found'});return;}
      Object.assign(row,input);
    }
    if(input.make_default){overview.default_policy_id=row.id;for(const p of overview.policies){p.enabled=false;p.next_run_at=undefined;}delete row.make_default;}
    if(row.enabled){overview.default_policy_id=row.id;for(const p of overview.policies){if(p!==row){p.enabled=false;p.next_run_at=undefined;}}row.next_run_at=new Date(Date.now()+row.interval_minutes*60000).toISOString();}
    else row.next_run_at=undefined;
    for(const p of overview.policies)p.is_default=p.id===overview.default_policy_id;
    const current=overview.policies.find(p=>p.is_default);
    overview.policy={enabled:current.enabled,interval_minutes:current.interval_minutes,retention_days:current.retention_days,retention_count:current.retention_count};
    overview.next_run_at=current.next_run_at;
    notify('snapshots');
    json(response,request.method==='POST'?201:200,{data:{policy:row}});return;
  }
  if (url.pathname === base + "/imports") {
    let body = "";
    for await (const chunk of request) body += chunk;
    requests[requests.length - 1].multipartFieldsValid =
      String(request.headers["content-type"]).startsWith(
        "multipart/form-data;",
      ) &&
      body.includes('name="file"') &&
      body.includes('name="password"');
    json(response, 200, { data: { snapshot } });
    return;
  }
  if (url.pathname.endsWith("/export") && url.pathname.startsWith(base + "/")) {
    response.writeHead(200, {
      "content-type": "application/octet-stream",
      "content-disposition": "attachment; filename=fixture.pxsnapshot",
      "cache-control": "no-store",
    });
    response.end("browser-only-download-fixture");
    return;
  }
  if (
    url.pathname.endsWith("/restores") &&
    url.pathname.startsWith(base + "/")
  ) {
    const restored = {
      id: "fbe50439-6c3e-4d1a-a277-03112363850a",
      snapshot_id: snapshot.id,
      status: "prepared",
      phase: "data_verified",
      created_at: new Date().toISOString(),
      directory: "/fixture/private/restore",
      database: "owned_browser_fixture",
      components: snapshot.components,
      business_verified: false,
      cutover_available: false,
      warnings: [
        "仅恢复到隔离数据副本；尚未启动应用或完成业务验收，未切换正式站",
      ],
    };
    overview.restores.unshift(restored);
    json(response, 202, { data: { restore: restored } });
    return;
  }
  if (
    url.pathname.endsWith("/protection") &&
    url.pathname.startsWith(base + "/")
  ) {
    overview.snapshots[0].protected = !overview.snapshots[0].protected;
    json(response, 200, { data: { snapshot: overview.snapshots[0] } });
    return;
  }
  if (url.pathname.endsWith("/setup/status")) {
    json(response, 200, {
      data: {
        configured: true,
        requires_login: true,
        restart_required: false,
        install_status: "installed",
      },
    });
    return;
  }
  if (url.pathname.endsWith("/user/auth/me/context")) {
    json(
      response,
      hasBearer ? 200 : 401,
      hasBearer ? { data: context } : { error: "missing bearer" },
    );
    return;
  }
  if (url.pathname.endsWith("/admin/menus")) {
    json(response, 200, {
      data: {
        categories: [
          {
            id: "ops",
            title: "运维",
            order: 1,
            children: [
              {
                id: "backup",
                title: "备份中心",
                path: "/ops/backup",
                origin: "core",
                visible: true,
              },
            ],
          },
        ],
      },
    });
    return;
  }
  json(response, 200, {
    data: {
      items: [],
      total: 0,
      policies: [],
      jobs: [],
      sources: [],
      alerts: [],
      history: [],
      runtime: { ready: true, problems: [] },
    },
  });
});
await new Promise((yes, no) => {
  backend.once("error", no);
  backend.listen(backendPort, "127.0.0.1", yes);
});

const child = spawn(process.execPath, [entry], {
  cwd: process.cwd(),
  stdio: ["ignore", "inherit", "inherit"],
  env: {
    ...process.env,
    NODE_ENV: "production",
    HOST: "127.0.0.1",
    PORT: String(webPort),
    POWERX_BACKEND: `http://127.0.0.1:${backendPort}`,
 POWERX_RECOVERY_STATUS_URL:`http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_API_BASE: "/api/v1",
    NUXT_PUBLIC_POWERX_CORE_BASE: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_UPSTREAM_ORIGIN: `http://127.0.0.1:${backendPort}`,
    NUXT_PUBLIC_WS_ORIGIN: `ws://127.0.0.1:${backendPort}`,
    NODE_PATH: [resolve("node_modules"), process.env.NODE_PATH]
      .filter(Boolean)
      .join(delimiter),
  },
});
const record = {
  owner: "snapshot-browser-tests",
  cwd: process.cwd(),
  startedAt: new Date().toISOString(),
  fixturePid: process.pid,
  fixturePort: backendPort,
  corePid: child.pid,
  corePort: webPort,
  command: [process.execPath, entry],
};
const resultsDir = resolve("../test-results");
mkdirSync(resultsDir, { recursive: true });
function saveRecord() {
  writeFileSync(
    resolve(resultsDir, "snapshot-browser-processes.json"),
    JSON.stringify(record, null, 2),
    { mode: 0o600 },
  );
}
saveRecord();
console.log(JSON.stringify(record));
let stopping = false;
function shutdown(signal) {
  if (stopping) return;
  stopping = true;
  record.stoppedAt = new Date().toISOString();
  saveRecord();
  backend.close();
  backend.closeAllConnections();
  if (child.exitCode !== null || child.signalCode !== null) {
    process.exitCode = child.exitCode || 0;
    return;
  }
  child.kill(signal);
  const force = setTimeout(() => child.kill("SIGKILL"), 5000);
  force.unref();
  child.once("exit", () => clearTimeout(force));
}
process.once("SIGTERM", () => shutdown("SIGTERM"));
process.once("SIGINT", () => shutdown("SIGINT"));
child.once("error", (error) => {
  console.error(error);
  shutdown("SIGTERM");
  process.exitCode = 1;
});
child.once("exit", (code) => {
  record.coreExitedAt = new Date().toISOString();
  saveRecord();
  if (!stopping) {
    shutdown("SIGTERM");
    process.exitCode = code || 1;
  }
});
