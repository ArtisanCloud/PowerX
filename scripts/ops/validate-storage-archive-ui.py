#!/usr/bin/env python3
from pathlib import Path
import argparse, contextlib, datetime, json, os, secrets, shutil, signal, socket, subprocess, time, urllib.request
import boto3
from botocore.config import Config

ROOT=Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser(description='Run native archive UI acceptance in owned MinIO and HTTP fixtures; never loads daily configuration')
parser.add_argument('--artifacts',type=Path,required=True)
parser.add_argument('--web-entry',default='.output-storage-archive-validation/server/index.mjs')
parser.add_argument('--minio-bin',default=shutil.which('minio'))
args=parser.parse_args()
if not args.minio_bin:parser.error('minio executable required')
if not (ROOT/'web-admin'/args.web_entry).is_file():parser.error('build the isolated Nuxt artifact first')
OUT=args.artifacts.resolve()
os.umask(0o077)
OUT.mkdir(mode=0o700,parents=True,exist_ok=False)
cfg={'Endpoint':'http://127.0.0.1:19400','Bucket':'powerx-archive-ui-'+secrets.token_hex(6),'Region':'AsiaShanghai','AccessKey':'pxui'+secrets.token_hex(8),'SecretKey':secrets.token_urlsafe(32)}
p=OUT/'s3.private.json';p.write_text(json.dumps(cfg));p.chmod(0o600)
processes=[];owned=[];logs=[];failure=None;browser_code=None;go_code=None;remote={};fixture={}
def save(name,value):
    p=OUT/name;p.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n');p.chmod(0o600)
def free(port):
    with socket.socket() as s:
        s.settimeout(.2);return s.connect_ex(('127.0.0.1',port))!=0
def start(command,cwd,env,log,ports):
    f=(OUT/log).open('wb');logs.append(f)
    p=subprocess.Popen(command,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT,start_new_session=True)
    processes.append({'pid':p.pid,'command':command,'cwd':str(cwd),'ports':ports});save('ownership.json',processes);owned.append(p);return p
def stop(p):
    if p.poll() is None:
        with contextlib.suppress(ProcessLookupError):os.killpg(p.pid,signal.SIGTERM)
        try:p.wait(timeout=15)
        except subprocess.TimeoutExpired:
            with contextlib.suppress(ProcessLookupError):os.killpg(p.pid,signal.SIGKILL)
            p.wait(timeout=10)
def interrupt(signum,_frame):raise InterruptedError(str(signum))
signal.signal(signal.SIGTERM,interrupt)
try:
    assert all(free(p) for p in [19400,19401,3487,3488]),'owned test ports must be free'
    for name in ['fixture.private.json','fixture.private.json.done']:
        p=OUT/name
        if p.exists():p.unlink()
    data=OUT/'minio-data';data.mkdir(mode=0o700)
    env=dict(os.environ,MINIO_ROOT_USER=cfg['AccessKey'],MINIO_ROOT_PASSWORD=cfg['SecretKey'],MINIO_REGION_NAME=cfg['Region'])
    minio=start([args.minio_bin,'server',str(data),'--address','127.0.0.1:19400','--console-address','127.0.0.1:19401'],ROOT,env,'minio.log',[19400,19401])
    deadline=time.monotonic()+20
    while True:
        try:
            with urllib.request.urlopen(cfg['Endpoint']+'/minio/health/live',timeout=1) as r:assert r.status==200
            break
        except Exception:
            if minio.poll() is not None or time.monotonic()>deadline:raise RuntimeError('owned MinIO did not start')
            time.sleep(.2)
    s3=boto3.client('s3',endpoint_url=cfg['Endpoint'],region_name=cfg['Region'],aws_access_key_id=cfg['AccessKey'],aws_secret_access_key=cfg['SecretKey'],config=Config(s3={'addressing_style':'path'},retries={'max_attempts':1}))
    s3.create_bucket(Bucket=cfg['Bucket'],CreateBucketConfiguration={'LocationConstraint':cfg['Region']})
    s3.put_bucket_versioning(Bucket=cfg['Bucket'],VersioningConfiguration={'Status':'Enabled'})
    native=OUT/'fixture.private.json'
    env=dict(os.environ,POWERX_ARCHIVE_UI_NATIVE_CONFIG=str(OUT/'s3.private.json'),POWERX_ARCHIVE_UI_NATIVE_FIXTURE=str(native))
    go=start(['go','test','-race','./internal/transport/http/admin/backup','-run','^TestArchiveNativeBrowserFixture$','-count=1','-v'],ROOT/'backend',env,'go-native.log',[])
    deadline=time.monotonic()+120
    while not native.exists():
        if go.poll() is not None:raise RuntimeError('native HTTP fixture failed to start')
        if time.monotonic()>deadline:raise RuntimeError('native HTTP fixture startup timeout')
        time.sleep(.25)
    fixture=json.loads(native.read_text())
    print(json.dumps({'native_fixture_ready':True,'minio_pid':minio.pid,'http_pid':fixture['pid']}),flush=True)
    env.update(POWERX_ARCHIVE_UI_ARTIFACTS=str(OUT/'browser'),POWERX_ARCHIVE_UI_WEB_ENTRY=args.web_entry)
    browser=start(['npx','playwright','test','-c','playwright.archive-native.config.ts'],ROOT/'web-admin',env,'browser.log',[3487,3488])
    browser_code=browser.wait(timeout=240)
    native.with_name(native.name+'.done').write_text('done\n')
    go_code=go.wait(timeout=25)
    versions=s3.list_object_versions(Bucket=cfg['Bucket']).get('Versions',[])
    remote={'bucket_versioning':s3.get_bucket_versioning(Bucket=cfg['Bucket']).get('Status'),'versions':len(versions),'bytes':sum(x.get('Size',0) for x in versions)}
    if browser_code or go_code:raise RuntimeError('native browser acceptance failed; inspect private logs')
except (Exception,KeyboardInterrupt) as e:failure=str(e)
finally:
    for p in reversed(owned):stop(p)
    for f in logs:f.close()
    cleanup={'all_recorded_processes_exited':all(p.poll() is not None for p in owned),'ports_released':all(free(p) for p in [19400,19401,3487,3488]),'owned_http_source_removed':not fixture or not Path(fixture['source']).exists(),'owned_http_listener_released':not fixture or free(int(fixture['url'].rsplit(':',1)[1]))}
    if cleanup['all_recorded_processes_exited'] and (OUT/'minio-data').exists():shutil.rmtree(OUT/'minio-data')
    cleanup['owned_minio_data_removed']=not (OUT/'minio-data').exists()
    if not all(cleanup.values()):failure=failure or 'owned fixture cleanup incomplete'
    result={'completed_at':datetime.datetime.now().astimezone().isoformat(),'passed':failure is None,'failure':failure,'browser_exit_code':browser_code,'native_exit_code':go_code,'remote':remote,'cleanup':cleanup}
    save('result.json',result);print(json.dumps(result,ensure_ascii=False),flush=True)
raise SystemExit(0 if failure is None else 1)
