# 固定协议的远端接收器；通过 SSH stdin 执行，不依赖 PowerX 或业务数据库。
import sys, os, json, hashlib, re, time, datetime, tempfile
from pathlib import Path
root_arg, instance, policy_id, job_id, operation = sys.argv[1:6]
ident = re.compile(r'^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$')
sha_re = re.compile(r'^[a-f0-9]{64}$')
if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,63}', instance) or not ident.fullmatch(policy_id): raise ValueError('invalid namespace')
if job_id != '-' and not ident.fullmatch(job_id): raise ValueError('invalid job')
root = Path(root_arg)
if not root.is_absolute() or root == Path('/'): raise ValueError('invalid archive root')
root.mkdir(parents=True, exist_ok=True, mode=0o700)
root = root.resolve()
namespace = root / instance / policy_id
namespace.mkdir(parents=True, exist_ok=True, mode=0o700)
if namespace.resolve() != namespace: raise ValueError('symlink namespace refused')
marker = namespace / '.powerx-archive.json'
expected = {'format': 'powerx.archive/v1', 'instance': instance, 'policy_id': policy_id}
if marker.exists():
    if marker.is_symlink() or json.loads(marker.read_text()) != expected: raise ValueError('namespace identity mismatch')
else:
    if any(namespace.iterdir()): raise ValueError('unowned nonempty namespace refused')
    with marker.open('x') as f: json.dump(expected,f)
    marker.chmod(0o600)
for name in ['objects', 'incoming', 'manifests']:
    p = namespace / name
    p.mkdir(exist_ok=True,mode=0o700)
    if p.resolve() != p: raise ValueError('symlink directory refused')
def regular(p):
    if p.is_symlink() or not p.is_file(): raise ValueError('regular file required')
    return p

def atomic(p, data):
    fd,name=tempfile.mkstemp(prefix='.archive-',dir=str(p.parent))
    try:
        with os.fdopen(fd,'wb') as f: f.write(data);f.flush();os.fsync(f.fileno())
        os.replace(name,p)
    finally:
        if os.path.exists(name): os.unlink(name)

def verify(manifest):
    if manifest.get('id') != job_id or manifest.get('policy_id') != policy_id or manifest.get('status') != 'success': raise ValueError('manifest identity mismatch')
    files=manifest.get('files',[])
    if not files or len(files)>1000000: raise ValueError('invalid file inventory')
    for entry in files:
        digest=entry['sha256'];name=entry['path']
        if not sha_re.fullmatch(digest) or name.startswith('/') or '\\' in name or any(v in ('','.','..') for v in name.split('/')): raise ValueError('unsafe entry')
        p=regular(namespace/'objects'/digest[:2]/digest)
        if p.parent.resolve()!=p.parent: raise ValueError('symlink object directory')
        h=hashlib.sha256();size=0
        with p.open('rb') as f:
            for block in iter(lambda:f.read(1024*1024),b''):h.update(block);size+=len(block)
        if h.hexdigest()!=digest or size!=entry['size']:raise ValueError('object checksum mismatch')

def cleanup(days,count):
    rows=[]
    for p in (namespace/'manifests').glob('*.json'):
        regular(p);m=json.loads(p.read_text());meta=m.get('_archive_remote',{})
        if m.get('policy_id')!=policy_id or not meta.get('committed_at'):raise ValueError('untracked manifest; cleanup refused')
        rows.append((float(meta['committed_at']),p,m))
    rows.sort(key=lambda x:x[0],reverse=True)
    for index,(stamp,p,m) in enumerate(rows):
        if index==0 or m.get('protected'):continue
        if index>=count or time.time()-stamp>days*86400:p.unlink()
    used=set()
    for p in (namespace/'manifests').glob('*.json'):
        for entry in json.loads(regular(p).read_text()).get('files',[]):used.add(entry['sha256'])
    for p in (namespace/'objects').glob('*/*'):
        if sha_re.fullmatch(p.name) and p.name not in used:regular(p).unlink()

result={'ok':True,'namespace':str(namespace),'configured_root':root_arg}
if operation=='init':pass
elif operation=='commit':
    incoming=regular(namespace/'incoming'/(job_id+'.json'));raw=incoming.read_bytes();manifest=json.loads(raw);verify(manifest)
    checksum=hashlib.sha256(raw).hexdigest();manifest['_archive_remote']={'committed_at':time.time(),'source_manifest_sha256':checksum}
    atomic(namespace/'manifests'/(job_id+'.json'),json.dumps(manifest,ensure_ascii=False).encode())
    incoming.unlink();days=int(sys.argv[6]);count=int(sys.argv[7])
    if days<1 or days>3650 or count<1 or count>10000:raise ValueError('invalid retention')
    cleanup(days,count);result.update(manifest_sha256=checksum,files=len(manifest['files']))
elif operation in ('read','verify'):
    manifest=json.loads(regular(namespace/'manifests'/(job_id+'.json')).read_text());verify(manifest)
    result['manifest']=manifest
elif operation=='protect':
    p=regular(namespace/'manifests'/(job_id+'.json'));manifest=json.loads(p.read_text());manifest['protected']=sys.argv[6]=='true';atomic(p,json.dumps(manifest,ensure_ascii=False).encode())
else:raise ValueError('unsupported operation')
print(json.dumps(result,ensure_ascii=False))
