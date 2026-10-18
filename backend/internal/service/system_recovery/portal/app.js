"use strict";
(() => {
  const base = "/__powerx_recovery/";
  const $ = id => document.getElementById(id);
  const streams = new Map();
  const requests = new Set();
  const authChannel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('powerx-recovery-auth');
  let csrf = "", catalog = null, selection = null, loading = false, refreshPending = false;
  let returnJobID = "", returnTimer = null, submitting = false, authenticated = false, authEpoch = 0;
  const states = { queued:"等待执行",running:"执行中",complete:"完成",failed:"失败",rolled_back:"已恢复原状态",attention_required:"需要核对现场",interrupted:"已中断",ready:"快照包可用",capturing:"正在采集",verifying:"正在校验",pruned:"内容已清理" };
  const phases = {failed:"恢复已停止",rolled_back:"已恢复到操作前状态",attention_required:"需要核对维护现场",protecting_current:"保存当前状态",replacing_database:"恢复当前数据库",verifying_startup:"重启并验证当前实例",queued:"等待执行",preflight_restore:"恢复预检查",maintenance:"进入维护",rollback_point_saved:"恢复前保护快照已保存",instance_replaced:"实例已覆盖，正在验证",resuming_writes:"正在启动业务",verifying_active:"核对正常运行状态",complete:"恢复完成",rolling_back:"正在恢复原状态",materializing:"展开恢复包",restoring_database:"恢复数据库",mapping_configuration:"恢复配置",verifying_application:"验证应用",capturing_frozen_components:"保存恢复前保护快照"};
  const active = job => ["queued","running"].includes(job.status);
  function node(tag, text, className) { const el=document.createElement(tag); if(text!==undefined)el.textContent=text; if(className)el.className=className; return el; }
  function notice(text) { $("message").textContent=text; $("message").hidden=!text; }
  function time(value) { return value ? new Date(value).toLocaleString() : "—"; }
  function size(value) { return value>=1048576 ? `${(value/1048576).toFixed(1)} MiB` : `${Math.ceil((value||0)/1024)} KiB`; }
  function action(text,callback,disabled=false) { const el=node("button",text);el.type="button";el.disabled=disabled;el.addEventListener("click",callback);return el; }
  function section(name) {
    for(const item of ["snapshots","jobs"]) { $(`${item}-panel`).hidden=item!==name;$(`${item}-tab`).setAttribute("aria-selected",String(item===name)); }
  }
  function stopStreams() { for(const stream of streams.values())stream.controller.abort();streams.clear(); }
  function lock() { authEpoch++;authenticated=false;csrf="";catalog=null;selection=null;returnJobID="";submitting=false;loading=false;refreshPending=false;for(const controller of requests)controller.abort();requests.clear();stopStreams();if(returnTimer)clearTimeout(returnTimer);returnTimer=null;if($("confirmation").open)$("confirmation").close();$("confirm-restore").disabled=false;$("cancel-restore").disabled=false;$("jobs").replaceChildren();$("snapshots").replaceChildren();$("workspace").hidden=true;$("admin-actions").hidden=true;$("login").hidden=false; }
  async function request(path, options={}) {
    const owner=authEpoch;
    const headers={...options.headers};
    if(options.body!==undefined){headers["Content-Type"]="application/json";options.body=JSON.stringify(options.body);}
    if(options.method && options.method!=="GET" && csrf)headers["X-PowerX-Recovery-CSRF"]=csrf;
    const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),15000);requests.add(controller);
    try {
      const response=await fetch(base+path,{credentials:"same-origin",cache:"no-store",...options,headers,signal:controller.signal});
      if(owner!==authEpoch)throw new Error("恢复身份已变化，请重新进入");
      if(!response.ok){let message="恢复服务暂不可用";if([502,503,504].includes(response.status))message="恢复服务正在重启或暂时无法连接，任务与当前页面已保留。进度连接恢复后将自动同步，请勿重复提交。";else try{message=(await response.json()).message||message;}catch{}if(response.status===401){lock();notice(message);}throw new Error(message);}
      return response;
    } finally {clearTimeout(timeout);requests.delete(controller);}
  }
  async function authenticatedWorkspace() {
    const owner=authEpoch;const session=await(await request("session")).json();if(owner!==authEpoch)return;csrf=session.csrf;authenticated=true;
    $("login").hidden=true;$("workspace").hidden=false;$("admin-actions").hidden=false;
    await loadCatalog();if(owner===authEpoch&&authenticated)startEvents();
  }
  async function loadCatalog() {
    if(!authenticated)return;
    if(loading){refreshPending=true;return;}
    loading=true;$("refresh").disabled=true;
    const owner=authEpoch;
    try {
      const value=await(await request("catalog")).json();if(!authenticated||owner!==authEpoch)return;
      // Preserve a newer independent progress event arriving during this read.
      const current=new Map((catalog?.jobs||[]).map(job=>[job.id,job]));
      value.jobs=value.jobs.map(job=>{const pushed=current.get(job.id);return pushed&&String(pushed.updated_at||"")>String(job.updated_at||"")?pushed:job;});
      catalog=value;
      if(!returnJobID){const pending=value.jobs.find(job=>active(job)&&["restore","resume"].includes(job.kind));if(pending){returnJobID=pending.id;section("jobs");}}
      render();syncProgress();
      if(value.executor_error)notice("恢复执行器需要运维处理，当前不能提交覆盖操作。恢复入口与认证仍可使用。详情："+value.executor_error);
      else if(value.restore_problem)notice(value.restore_problem);
      else if(value.startup_error)notice("业务启动失败，维护恢复仍在线。可从有效快照发起恢复预检查。详情："+value.startup_error);
      else if(value.maintenance)notice("系统正在维护。此页面可刷新或重新打开，业务完成恢复后将自动启动。");
      else notice("");
    } catch(error){if(owner===authEpoch)notice(error.message);} finally {if(owner===authEpoch){loading=false;$("refresh").disabled=false;if(refreshPending){refreshPending=false;void loadCatalog();}}}
  }
  function blocked() { return !catalog || !catalog.executor_online || !!catalog.executor_error || catalog.maintenance || catalog.jobs.some(job=>active(job)||job.status==="attention_required"); }
  function renderSnapshots() {
    $("snapshots").replaceChildren();$("empty-snapshots").hidden=!!catalog.snapshots.length;
    $("business-status").textContent=catalog.maintenance?"维护中":catalog.business_online?"业务运行中":"业务已停止";
    for(const snapshot of catalog.snapshots) {
      const row=node("tr"),point=node("td",time(snapshot.captured_at||snapshot.created_at));point.append(node("small",snapshot.id));if(snapshot.protected)point.append(node("small","已保护"));row.append(point,node("td",snapshot.version),node("td",states[snapshot.status]||snapshot.status),node("td",size(snapshot.size_bytes)));
      const cell=node("td"),button=action("恢复当前实例",()=>openConfirmation(snapshot),blocked()||!snapshot.allowed);button.className="primary";
      if(!snapshot.allowed)button.title="此包不可用或不符合当前实例恢复合同";cell.append(button);row.append(cell);$("snapshots").append(row);
    }
  }
  function renderJobs() {
    const expanded=new Set([...$("jobs").querySelectorAll("details[open]")].map(el=>el.dataset.job));
    $("jobs").replaceChildren();const jobs=catalog.jobs.filter(job=>["restore","resume"].includes(job.kind)||active(job));$("empty-jobs").hidden=!!jobs.length;
    for(const job of jobs) {
      const article=node("article",undefined,"job"),top=node("div",undefined,"job-top");top.append(node("h3",`${time(job.created_at)} · ${job.kind==="resume"?"继续当前实例":"恢复任务"}`),node("span",states[job.status]||job.status,"badge"));article.append(top,node("p",job.id,"muted"),node("p",phases[job.phase]||job.phase));
      if(job.error)article.append(node("p",job.error,"error"));
      if(job.rollback_snapshot_id){article.append(node("p","恢复前保护快照："+job.rollback_snapshot_id,"muted"));const protection=catalog.snapshots.find(snapshot=>snapshot.id===job.rollback_snapshot_id);if(protection?.allowed)article.append(action("恢复这份保护快照",()=>openConfirmation(protection),blocked()));}
      if(job.status==="attention_required")article.append(action("继续当前实例",()=>openConfirmation(null,job),catalog.jobs.some(other=>active(other))));
      const details=node("details"),summary=node("summary","查看报告");details.dataset.job=job.id;details.open=expanded.has(job.id);details.append(summary,node("pre",(job.checks||[]).map(check=>`${check.passed?"✓":"×"} ${check.name}\n${check.detail}`).join("\n\n")||"暂无检查结果"));article.append(details);$("jobs").append(article);
    }
  }
  function render() { if(!catalog)return;renderSnapshots();renderJobs(); }
  function applyProgress(job) {
    if(!catalog||!authenticated)return;
    const prior=catalog.jobs.find(value=>value.id===job.id);if(prior&&String(prior.updated_at||"")>String(job.updated_at||""))return;
    catalog.jobs=[job,...catalog.jobs.filter(value=>value.id!==job.id)];
    if(job.status==="complete"&&["restore","resume"].includes(job.kind)) {
      catalog.maintenance=false;notice("恢复完成，业务已启动。可以返回站点核对数据。");
      if(job.id===returnJobID&&$("auto-return").checked&&!returnTimer) returnTimer=setTimeout(()=>location.replace("/ops/backup"),3000);
      void loadCatalog();
    }
    else if(!active(job))void loadCatalog();
    render();syncProgress();
  }
  function pause(ms,signal) { return new Promise(resolve=>{if(signal.aborted){resolve();return;}const timer=setTimeout(done,ms);function done(){clearTimeout(timer);signal.removeEventListener("abort",done);resolve();}signal.addEventListener("abort",done,{once:true});}); }
  async function consume(body,callback,signal) {
    const reader=body.getReader(),decoder=new TextDecoder();let pending="";
    try { while(!signal.aborted){const {done,value}=await reader.read();if(done)break;pending+=decoder.decode(value,{stream:true});pending=pending.replace(/\r\n/g,"\n");let end;while((end=pending.indexOf("\n\n"))!==-1){const block=pending.slice(0,end);pending=pending.slice(end+2);let event="message";const data=[];for(const line of block.split("\n")){if(line.startsWith("event:"))event=line.slice(6).trim();if(line.startsWith("data:"))data.push(line.slice(5).trimStart());}if(data.length){let payload;try{payload=JSON.parse(data.join("\n"));}catch{continue;}callback(event,payload);}}} } finally {try{await reader.cancel();}catch{}}
  }
  function connect(name,path,ticket,callback) {
    if(streams.has(name)||!authenticated)return;
    const owner=authEpoch,controller=new AbortController(),stream={controller,ticket};streams.set(name,stream);
    void(async()=>{
      let failures=0,reconnected=false;
      while(failures<8&&!controller.signal.aborted) {
        const started=Date.now();let authorizedRotation=false;
        try {
          const headers={Accept:"text/event-stream"};if(ticket)headers.Authorization="Bearer "+ticket;
          const response=await fetch(path,{headers,credentials:ticket?"omit":"same-origin",cache:"no-store",signal:controller.signal});
          if(response.status===401){if(ticket){controller.abort();streams.delete(name);void loadCatalog();}else{lock();notice("恢复管理会话已失效，请重新验证身份。");}return;}
          if(!response.ok||!response.body||!response.headers.get("content-type")?.includes("text/event-stream"))throw new Error("进度连接暂不可用");
          if(reconnected)void loadCatalog();
          await consume(response.body,(event,data)=>{if(event==="reconnect"){authorizedRotation=true;return;}if(owner===authEpoch&&authenticated&&!controller.signal.aborted)callback(event,data);},controller.signal);
          if(authorizedRotation){failures=0;reconnected=true;continue;}
          if(!controller.signal.aborted)throw new Error("连接已断开");
        } catch(error) {if(controller.signal.aborted)break;if(Date.now()-started>10000)failures=0;notice("进度连接中断，正在尝试重新连接。任务与表单已保留，请勿重复提交。");reconnected=true;await pause(Math.min(500*2**failures,4000),controller.signal);failures++;}
      }
      if(streams.get(name)===stream)streams.delete(name);
      if(!controller.signal.aborted&&authenticated)notice("进度连接尚未恢复。请点击刷新重新连接；正在执行的任务不会因此取消。");
    })();
  }
  function startEvents() { connect("resources",base+"events","",event=>{if(event==="invalidate")void loadCatalog();}); }
  function syncProgress() {
    const wanted=new Set(catalog.jobs.filter(active).map(job=>job.id));
    for(const [name,stream] of streams)if(name!=="resources"&&!wanted.has(name)){stream.controller.abort();streams.delete(name);}
    for(const id of wanted){const ticket=catalog.progress_tokens[id];if(ticket)connect(id,base+"jobs/"+encodeURIComponent(id)+"/events",ticket,(event,data)=>{if(event==="progress"&&data.recovery?.id===id)applyProgress(data.recovery);});}
  }
  function openConfirmation(snapshot,job) {
    selection={snapshot,job,key:crypto.randomUUID(),generation:catalog.generation};$("consent").checked=false;$("confirmation-error").textContent="";
    $("confirmation-title").textContent=job?"继续当前实例":"恢复当前实例";
    $("selected-snapshot").textContent=job?`任务 ${job.id}`:`${time(snapshot.captured_at||snapshot.created_at)} · ${snapshot.id}`;
    $("confirmation-description").textContent=job?"重新验证当前维护现场的数据并启动业务，不重新覆盖数据库。验证失败将继续保持维护。":"将覆盖数据库、媒体、配置和配套程序。系统会先保护当前状态，恢复完成后自动启动业务。";
    $("confirmation").showModal();
  }
  $("restore-form").addEventListener("submit",async event=>{
    event.preventDefault();if(!selection||!$("consent").checked||submitting)return;
    submitting=true;$("confirm-restore").disabled=true;$("cancel-restore").disabled=true;$("confirmation-error").textContent="";
    const chosen=selection;
    try {
      const body=chosen.job?{job_id:chosen.job.id,confirmation:chosen.job.id,generation:chosen.generation}:{snapshot_id:chosen.snapshot.id,confirmation:chosen.snapshot.id,generation:chosen.generation};
      const receipt=await(await request(chosen.job?"resume":"restore",{method:"POST",body,headers:{"Idempotency-Key":chosen.key}})).json();
      if(!authenticated||selection!==chosen||!catalog)return;
      catalog.progress_tokens[receipt.recovery.id]=receipt.progress_token;returnJobID=receipt.recovery.id;applyProgress(receipt.recovery);$("confirmation").close();selection=null;section("jobs");notice("任务已提交，正在进入维护恢复。您可以刷新此页接续进度。");
    } catch(error){if(selection===chosen)$("confirmation-error").textContent=error.message;} finally {if(selection===chosen||!selection){submitting=false;$("confirm-restore").disabled=false;$("cancel-restore").disabled=false;}}
  });
  $("cancel-restore").addEventListener("click",()=>{if(!submitting){$("confirmation").close();selection=null;}});
  $("confirmation").addEventListener("cancel",event=>{if(submitting)event.preventDefault();else selection=null;});
  $("login-form").addEventListener("submit",async event=>{
    event.preventDefault();const input=$("recovery-key"),key=input.value;input.value="";const button=event.submitter;button.disabled=true;
    try{await request("session",{method:"POST",body:{key}});notice("");await authenticatedWorkspace();}catch(error){notice(error.message);}finally{button.disabled=false;}
  });
  $("refresh").addEventListener("click",()=>{void loadCatalog();startEvents();syncProgress();});
  $("logout").addEventListener("click",async()=>{try{await request("session",{method:"DELETE"});lock();authChannel?.postMessage({event:"logout"});notice("已退出独立恢复会话。");}catch(error){notice(error.message);}});
  $("download-key").addEventListener("click",async()=>{try{const response=await request("recovery-key",{method:"POST"}),blob=await response.blob(),url=URL.createObjectURL(blob),link=node("a");link.href=url;link.download="powerx-recovery-administrator.key";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);notice("恢复密钥已下载。请妥善保存；业务停止、Core 处于维护模式时可用它重新认证。");}catch(error){notice(error.message);}});
  $("auto-return").addEventListener("change",()=>{if(!$("auto-return").checked&&returnTimer){clearTimeout(returnTimer);returnTimer=null;}});
  for(const name of ["snapshots","jobs"])$(`${name}-tab`).addEventListener("click",()=>section(name));
  if(authChannel)authChannel.onmessage=event=>{if(event.data?.event==="logout"){lock();notice("管理员已退出，请重新验证恢复身份。");}};
  window.addEventListener("pagehide",()=>{stopStreams();authChannel?.close();if(returnTimer)clearTimeout(returnTimer);});
  void authenticatedWorkspace().catch(error=>{lock();notice(error.message);});
})();
