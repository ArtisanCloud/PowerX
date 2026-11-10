#!/usr/bin/env python3
"""只读盘点旧执行器和交接前置条件。报告不是切换许可；本工具没有 apply 操作。"""
import argparse
import ast
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import stat
import subprocess


SCHEMA = 'powerx.executor-handoff/v1'
PROGRAM_DIRECTORY = '/opt/powerx-ops/space-guard'
PROGRAMS = {'guard.py': 'file_retention', 'audit_custody.py': 'audit_custody',
            'custody_policy.py': 'custody_binding', 'run_receiver.py': 'archive_receiver'}
PROTOCOL_PROGRAMS = {'guard.py', 'audit_custody.py', 'custody_policy.py'}
KNOWN_UNITS = ('powerx-space-guard.timer', 'powerx-space-guard.service')
UNIT_DIRECTORIES = ('/etc/systemd/system', '/usr/lib/systemd/system', '/lib/systemd/system')
CRON_DIRECTORIES = ('/etc/cron.d', '/etc/cron.hourly', '/etc/cron.daily',
                    '/etc/cron.weekly', '/etc/cron.monthly',
                    '/var/spool/cron/crontabs', '/var/spool/cron')
UNIT_NAME = re.compile(r'^[A-Za-z0-9][A-Za-z0-9_.@:-]*\.(?:service|timer)$')


class InspectionError(Exception):
    pass


def timestamp(value):
    # Go time.Time 输出 RFC3339 的 Z／纳秒；规范化后兼容服务器 Python 3.9/3.10。
    if not isinstance(value, str):
        raise InspectionError('timestamp_invalid')
    match = re.fullmatch(r'(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})', value)
    if not match:
        raise InspectionError('timestamp_invalid')
    whole, fraction, offset = match.groups()
    if fraction:
        whole += '.' + fraction[:6].ljust(6, '0')
    return dt.datetime.fromisoformat(whole + ('+00:00' if offset == 'Z' else offset))


def program_roles(raw):
    # 只返回固定类别，不能将命令行、crontab、Environment 或密钥写入报告。
    return sorted({role for name, role in PROGRAMS.items() if name.encode() in raw})


class Filesystem:
    def __init__(self, root=Path('/'), snapshot=False):
        self.root = root.resolve(strict=True)
        self.snapshot = snapshot

    def path(self, name, allow_links=False):
        if not isinstance(name, str) or not name.startswith('/') or str(Path(name)) != name or '..' in Path(name).parts:
            raise InspectionError('path_invalid')
        path = self.root / name.lstrip('/')
        # 快照中的链接不能把只读探测重定向到真实主机或根目录以外。
        try:
            if path.resolve(strict=True) != path:
                if not allow_links:
                    raise InspectionError('path_link_requires_review')
                resolved = path.resolve(strict=True)
                if self.snapshot and not resolved.is_relative_to(self.root):
                    raise InspectionError('path_link_requires_review')
                path = resolved
        except FileNotFoundError:
            pass
        return path

    def read(self, name, limit=1 << 20, owner=None, private=False, allow_links=False):
        path = self.path(name, allow_links=allow_links)
        flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
        fd = os.open(path, flags)
        try:
            before = os.fstat(fd)
            if (not stat.S_ISREG(before.st_mode) or before.st_nlink != 1
                    or before.st_size > limit or before.st_mode & (0o077 if private else 0o022)
                    or owner is not None and before.st_uid != owner):
                raise InspectionError('file_identity_invalid')
            chunks, size = [], 0
            while True:
                chunk = os.read(fd, min(65536, limit + 1 - size))
                if not chunk:
                    break
                chunks.append(chunk)
                size += len(chunk)
                if size > limit:
                    raise InspectionError('file_too_large')
            after = os.fstat(fd)
            if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns):
                raise InspectionError('file_changed_during_inspection')
            raw = b''.join(chunks)
            return raw, {'path': name, 'uid': before.st_uid, 'mode': oct(stat.S_IMODE(before.st_mode)),
                         'sha256': hashlib.sha256(raw).hexdigest()}
        finally:
            os.close(fd)

    def entries(self, directory, limit=256, allow_links=False):
        path = self.path(directory, allow_links=allow_links)
        if not path.exists():
            return []
        if not path.is_dir():
            raise InspectionError('directory_invalid')
        found = []
        with os.scandir(path) as rows:
            for row in rows:
                found.append(row.name)
                if len(found) > limit:
                    raise InspectionError('directory_scan_limit')
        return sorted(found)

    def masked_unit(self, name, owner):
        original = self.root / name.lstrip('/')
        target = self.path(name, allow_links=True)
        if target != self.root / 'dev/null' or original.lstat().st_uid != owner:
            return False
        metadata = target.stat()
        return stat.S_ISCHR(metadata.st_mode) and metadata.st_uid == 0


class Host:
    def unit(self, name):
        if not UNIT_NAME.fullmatch(name):
            return {'name': name, 'state': 'unknown', 'problem': 'unit_name_invalid'}
        command = next((p for p in ['/usr/bin/systemctl', '/bin/systemctl'] if Path(p).is_file()), None)
        if command is None:
            return {'name': name, 'state': 'unknown', 'problem': 'systemd_unavailable'}
        try:
            result = subprocess.run([command, 'show', '--no-pager', '--property=LoadState,ActiveState,SubState,UnitFileState,MainPID,Result,ExecMainCode,ExecMainStatus', '--', name],
                                    env={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C', 'SYSTEMD_PAGER': ''},
                                    capture_output=True, timeout=5)
        except (OSError, subprocess.TimeoutExpired):
            return {'name': name, 'state': 'unknown', 'problem': 'systemd_query_failed'}
        if len(result.stdout) > 16384:
            return {'name': name, 'state': 'unknown', 'problem': 'systemd_response_invalid'}
        values = dict(line.split('=', 1) for line in result.stdout.decode(errors='replace').splitlines() if '=' in line)
        # 退出失败不能当作 service 不存在；D-Bus、权限错误同样必须显示 unknown。
        if result.returncode or values.get('LoadState') not in {'loaded', 'not-found', 'masked', 'error', 'bad-setting', 'merged', 'stub'}:
            return {'name': name, 'state': 'unknown', 'problem': 'systemd_query_failed'}
        selected = {k: values[k] for k in ['LoadState', 'ActiveState', 'SubState', 'UnitFileState', 'MainPID', 'Result', 'ExecMainCode', 'ExecMainStatus'] if k in values}
        if values['LoadState'] == 'not-found':
            state = 'not_found'
        elif values['LoadState'] not in {'loaded', 'masked'}:
            state = 'unknown'
        elif values.get('ActiveState') == 'failed':
            state = 'failed'
        elif values.get('ActiveState') in {'active', 'activating', 'reloading', 'deactivating'}:
            state = 'active'
        elif values.get('UnitFileState') not in {'disabled', 'masked', 'masked-runtime', 'static', 'indirect', 'generated', 'transient', ''}:
            state = 'scheduled'
        elif values.get('ActiveState') == 'inactive':
            state = 'inactive'
        else:
            state = 'unknown'
        return {'name': name, 'state': state, 'properties': selected}

    def processes(self):
        rows, problems = [], []
        try:
            pids = sorted((p for p in Path('/proc').iterdir() if p.name.isdigit()), key=lambda p: int(p.name))
        except OSError:
            return [], ['process_scan_unavailable']
        if len(pids) > 4096:
            return [], ['process_scan_limit']
        for path in pids:
            try:
                # 不输出原始命令。/proc 的虚拟文件不能使用磁盘文件 size=0 作 EOF 判断。
                with (path / 'cmdline').open('rb') as handle:
                    raw = handle.read((1 << 20) + 1)
                if len(raw) > 1 << 20:
                    problems.append('process_command_limit')
                    continue
                roles = program_roles(raw)
                executable = raw.split(b'\0', 1)[0].rsplit(b'/', 1)[-1]
                if executable in {b'pg_dump', b'pg_restore'}:
                    roles.append('database_job_scope_requires_review')
                if executable in {b'powerx-archive-worker', b'archive-worker'}:
                    roles.append('archive_worker_scope_requires_review')
                if not roles:
                    continue
                record = (path / 'stat').read_text()
                rest = record[record.rfind(')') + 2:].split()
                rows.append({'pid': int(path.name), 'ppid': int(rest[1]),
                             'start_ticks': int(rest[19]), 'uid': path.stat().st_uid,
                             'roles': sorted(set(roles))})
            except FileNotFoundError:
                continue  # 进程在本次只读遍历中退出；不宣称全机原子快照。
            except (OSError, ValueError, IndexError):
                problems.append('process_scan_incomplete')
        return rows, sorted(set(problems))


def guard_run(fs, owner, now):
    """systemd exit=2 也可能是容量 critical；保留原始分类而不声称脚本崩溃。"""
    try:
        raw, _ = fs.read('/var/lib/powerx-space-guard/latest.json', owner=owner, private=True)
        data = json.loads(raw)
        if not isinstance(data, dict):
            raise InspectionError('legacy_run_document_invalid')
        if data.get('status') not in {'ok', 'warning', 'critical', 'failed', 'skipped', 'unconfirmed'}:
            raise InspectionError('legacy_run_status_unknown')
        finished = timestamp(data['finished_at'])
        if finished.tzinfo is None:
            raise InspectionError('legacy_run_time_invalid')
        out = {'state': 'observed', 'status': data['status'], 'finished_at': finished.isoformat(),
               'fresh': -30 <= (now - finished).total_seconds() <= 900}
        if isinstance(data.get('errors'), list):
            out['error_count'] = len(data['errors'])
        after = data.get('after')
        if isinstance(after, dict):
            available, ratio = after.get('available_bytes'), after.get('used_ratio')
            if type(available) is int and available >= 0:
                out['available_bytes'] = available
            if type(ratio) in {int, float} and 0 <= ratio <= 1:
                out['used_ratio'] = ratio
        return out
    except (OSError, ValueError, TypeError, KeyError, InspectionError):
        return {'state': 'unknown', 'problem': 'legacy_run_unavailable'}


def inspect(fs, host=None, *, binding_file='/etc/powerx-space-guard/core-binding.json',
            control_file=None, program_dir=PROGRAM_DIRECTORY, owner=0, now=None):
    now = now or dt.datetime.now(dt.timezone.utc)
    problems, programs, cron, units, sources = [], [], [], [], []

    def issue(code, subject):
        problems.append({'code': code, 'subject': subject})

    def read_json(name, uid=None, private=False):
        raw, meta = fs.read(name, owner=uid, private=private)
        value = json.loads(raw)
        if not isinstance(value, dict):
            raise InspectionError('document_invalid')
        return value, meta

    for name, role in PROGRAMS.items():
        path = program_dir + '/' + name
        item = {'path': path, 'role': role, 'state': 'missing'}
        try:
            raw, metadata = fs.read(path, owner=owner)
            item.update(metadata, state='present')
            if name in PROTOCOL_PROGRAMS:
                tree = ast.parse(raw)
                protocol = any(isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'CONTROL_PROTOCOL' for t in n.targets)
                               and isinstance(n.value, ast.Constant) and type(n.value.value) is int and n.value.value == 2 for n in tree.body)
                item['control_protocol'] = 2 if protocol else None
                if not protocol:
                    issue('legacy_program_upgrade_required', path)
        except FileNotFoundError:
            pass
        except (OSError, ValueError, SyntaxError, InspectionError):
            item['state'] = 'unknown'
            issue('program_unverified', path)
        programs.append(item)

    names, cron_paths = set(KNOWN_UNITS), {'/etc/crontab'}
    for directory in CRON_DIRECTORIES:
        try:
            cron_paths.update(directory + '/' + name for name in fs.entries(directory))
        except (OSError, InspectionError):
            issue('cron_scan_incomplete', directory)
    for path in sorted(cron_paths):
        try:
            if fs.path(path).is_dir():
                continue  # 独立列出的 crontab 目录会另行扫描，不读取 at 队列或未知目录。
            raw, metadata = fs.read(path)
            hits = []
            for number, line in enumerate(raw.splitlines(), 1):
                if line.lstrip().startswith(b'#'):
                    continue
                roles = program_roles(line)
                if roles:
                    hits.append({'line': number, 'roles': roles})
            if hits:
                cron.append(dict(metadata, matches=hits))
                issue('legacy_cron_requires_review', path)
        except FileNotFoundError:
            pass
        except (OSError, InspectionError):
            issue('cron_scan_incomplete', path)

    # 除标准 unit 名称外，识别包装同一脚本的部署方自定义 service 与其 timer。
    discovered, masked_units = [], set()
    visited_unit_directories = set()
    for directory in UNIT_DIRECTORIES:
        try:
            actual = fs.path(directory, allow_links=True)
            if not actual.exists():
                continue
            metadata = actual.stat()
            identity = (metadata.st_dev, metadata.st_ino)
            if identity in visited_unit_directories:
                continue
            visited_unit_directories.add(identity)
            for name in fs.entries(directory, limit=2048, allow_links=True):
                if UNIT_NAME.fullmatch(name):
                    path = directory + '/' + name
                    try:
                        if fs.masked_unit(path, owner):
                            masked_units.add(name)
                            continue
                        raw, _ = fs.read(path, owner=owner, allow_links=True)
                        discovered.append((name, raw))
                        if program_roles(raw):
                            names.add(name)
                    except (OSError, InspectionError):
                        issue('unit_file_requires_review', path)
                elif name.endswith('.d') and UNIT_NAME.fullmatch(name[:-2]):
                    try:
                        for fragment in fs.entries(directory + '/' + name, 64, allow_links=True):
                            if fragment.endswith('.conf'):
                                raw, _ = fs.read(directory + '/' + name + '/' + fragment, owner=owner, allow_links=True)
                                if program_roles(raw):
                                    names.add(name[:-2])
                    except (OSError, InspectionError):
                        issue('unit_override_requires_review', directory + '/' + name)
        except (OSError, InspectionError):
            issue('unit_scan_incomplete', directory)
    for name, raw in discovered:
        if name.endswith('.timer') and any(unit.encode() in raw for unit in names if unit.endswith('.service')):
            names.add(name)
    query_names = sorted(names)
    if len(query_names) > 64:
        issue('unit_query_limit', 'systemd')
        query_names = sorted(set(KNOWN_UNITS) | set(sorted(names - set(KNOWN_UNITS))[:62]))
    for name in query_names:
        state = host.unit(name) if host else {'name': name, 'state': 'unknown', 'problem': 'snapshot_not_live'}
        units.append(state)
        if state['state'] in {'active', 'scheduled'}:
            issue('legacy_schedule_requires_review', name)
        elif state['state'] == 'failed':
            issue('legacy_service_failed', name)
        elif state['state'] == 'unknown':
            issue('unit_state_unknown', name)
    processes, process_problems = host.processes() if host else ([], ['snapshot_not_live'])
    for code in process_problems:
        issue(code, 'processes')
    for process in processes:
        issue('active_executor_requires_review', str(process['pid']))
    latest = guard_run(fs, owner, now)
    if latest['state'] == 'observed':
        if not latest['fresh']:
            issue('legacy_run_stale', 'guard')
        elif latest['status'] in {'critical', 'failed', 'unconfirmed'}:
            issue('legacy_run_' + latest['status'], 'guard')
    elif any(p['role'] == 'file_retention' and p['state'] == 'present' for p in programs):
        issue('legacy_run_unavailable', 'guard')

    binding, binding_meta, control_meta = {}, {}, {}
    binding_state, control_state, identity = 'unavailable', 'unavailable', {}
    try:
        binding, binding_meta = read_json(binding_file, owner)
        if binding.get('protocol') != 2 or type(binding.get('core_uid')) is not int or not 0 <= binding['core_uid'] < 2 ** 32 or not isinstance(binding.get('node_id'), str) or not re.fullmatch(r'[A-Za-z0-9_.:-]{1,128}', binding['node_id']):
            raise InspectionError('binding_invalid')
        if not isinstance(binding.get('sources'), list) or len(binding['sources']) > 64:
            raise InspectionError('binding_sources_invalid')
        for path in binding['sources']:
            fs.path(path)
        if len(set(binding['sources'])) != len(binding['sources']):
            raise InspectionError('duplicate_bound_source')
        for field, name in [('guard', 'guard.py'), ('helper', 'audit_custody.py'), ('policy', 'custody_policy.py')]:
            item = next(p for p in programs if p['path'] == program_dir + '/' + name)
            if binding.get(field + '_script') != item['path'] or item['state'] != 'present' or binding.get(field + '_sha256') != item['sha256'] or item.get('control_protocol') != 2:
                raise InspectionError('binding_program_mismatch')
        if control_file and control_file != binding.get('control_file'):
            raise InspectionError('binding_control_mismatch')
        binding_state = 'verified'
        identity = {'node_id': binding['node_id'], 'core_uid': binding['core_uid']}
        control_file = binding.get('control_file')
        control, control_meta = read_json(control_file, binding['core_uid'], private=True)
        if control.get('protocol') != 2 or control.get('node_id') != binding['node_id']:
            raise InspectionError('control_identity_mismatch')
        published = timestamp(control['published_at'])
        until = timestamp(control['valid_until'])
        if published.tzinfo is None or until.tzinfo is None or not published <= now + dt.timedelta(seconds=30) or not now < until <= published + dt.timedelta(seconds=300):
            raise InspectionError('control_expired')
        entries = control.get('sources')
        if not isinstance(entries, list) or len(entries) > 64:
            raise InspectionError('control_sources_invalid')
        seen = set()
        for entry in entries:
            if not isinstance(entry, dict) or not isinstance(entry.get('source_id'), str) or len(entry['source_id']) > 128 or entry.get('executor') != 'powerx' or type(entry.get('archive_enabled')) is not bool:
                raise InspectionError('control_source_invalid')
            directory = entry.get('directory')
            path = fs.path(directory)
            if directory in seen:
                raise InspectionError('duplicate_source')
            seen.add(directory)
            source = {'source_id': entry['source_id'], 'directory': directory,
                      'bound': directory in binding['sources'], 'archive_enabled': entry['archive_enabled'],
                      'quarantine': 'unknown'}
            metadata = path.stat()
            if not stat.S_ISDIR(metadata.st_mode):
                raise InspectionError('source_not_directory')
            source['identity'] = {'device': metadata.st_dev, 'inode': metadata.st_ino, 'uid': metadata.st_uid}
            try:
                pending = fs.entries(directory + '/.powerx-audit-custody', 1000)
                source['quarantine'] = 'pending' if pending else 'empty_or_absent'
                source['quarantine_entries'] = len(pending)
                if pending:
                    issue('legacy_quarantine_requires_reconciliation', directory)
            except (OSError, InspectionError):
                issue('legacy_quarantine_unknown', directory)
            if not source['bound']:
                issue('source_not_bound', directory)
            sources.append(source)
        if set(binding['sources']) - seen:
            raise InspectionError('bound_source_missing_from_core')
        control_state = 'verified'
    except FileNotFoundError:
        issue('binding_or_control_missing', binding_file)
    except (OSError, ValueError, TypeError, KeyError, InspectionError) as error:
        code = str(error) if isinstance(error, InspectionError) else 'binding_or_control_unverified'
        issue(code, 'binding_and_control')

    basis = {'identity': identity, 'binding_state': binding_state, 'control_state': control_state,
             'masked_unit_files': sorted(masked_units),
             'programs': programs, 'units': units, 'cron': cron, 'processes': processes,
             'binding': binding_meta, 'control': control_meta, 'sources': sources, 'guard_run': latest, 'problems': problems}
    digest = hashlib.sha256(json.dumps(basis, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
    summary = {'programs_present': sum(p['state'] == 'present' for p in programs),
               'active_or_scheduled_units': sum(u['state'] in {'active', 'scheduled'} for u in units),
               'failed_units': sum(u['state'] == 'failed' for u in units),
               'cron_matches': sum(len(row['matches']) for row in cron),
               'visible_active_executors': len(processes), 'registered_sources': len(sources),
               'pending_quarantine_sources': sum(row['quarantine'] == 'pending' for row in sources),
               'review_items': len(problems)}
    return {'schema': SCHEMA, 'observed_at': now.isoformat(), 'mode': 'snapshot' if fs.snapshot else 'live_linux',
            'observer': {'pid': os.getpid(), 'cwd': os.getcwd(), 'ports': []},
            'read_only': True, 'authorizes_switch': False, 'assessment': 'requires_review', 'basis_sha256': digest,
            'summary': summary,
            'coverage': ['known PowerX guard programs, system units and bounded cron directories',
                         'visible current processes; not an atomic process snapshot',
                         'registered source identities and quarantine entry counts',
                         'does not scan at jobs, containers or user systemd managers',
                         'does not prove unknown wrappers, other hosts, admitted-job drain or new Core publication'],
            **basis,
            'next_steps': ['finish long-lived remote acceptance before production handoff',
                           'enter Core handoff latch and pause every registered automatic policy; wait for publication',
                           'reconcile each legacy source and unknown quarantine result; verify admitted jobs separately',
                           'review partial source coverage before stopping any shared guard or timer',
                           'stop only explicitly selected old executors in an authorized maintenance step and re-run inventory',
                           'complete Core handoff while paused, then separately resume and enable reviewed policies']}


def compare(previous, current):
    if previous.get('schema') != SCHEMA or previous.get('authorizes_switch') is not False:
        raise InspectionError('comparison_report_invalid')
    sections = ['identity', 'binding_state', 'control_state', 'masked_unit_files', 'programs', 'units', 'cron', 'processes', 'binding', 'control', 'sources', 'guard_run', 'problems']
    changed = [section for section in sections if previous.get(section) != current.get(section)]
    return {'same_basis': not changed, 'changed_sections': changed, 'authorizes_switch': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, help='只读导出树；使用该参数始终标为 snapshot，不探测真实进程／systemd')
    parser.add_argument('--binding-file', default='/etc/powerx-space-guard/core-binding.json')
    parser.add_argument('--control-file', help='核对已绑定控制文件，不能替换绑定中的路径')
    parser.add_argument('--program-dir', default=PROGRAM_DIRECTORY)
    parser.add_argument('--compare', type=Path, help='对比上次报告；任何对比结果都不授权切换')
    parser.add_argument('--output', type=Path, help='可选新建 0600 证据文件，不覆盖现有文件')
    args = parser.parse_args()
    snapshot = args.root is not None or platform.system() != 'Linux'
    fs = Filesystem(args.root or Path('/'), snapshot=snapshot)
    report = inspect(fs, None if snapshot else Host(), binding_file=args.binding_file,
                     control_file=args.control_file, program_dir=args.program_dir)
    if args.compare:
        with args.compare.open('rb') as handle:
            raw = handle.read((8 << 20) + 1)
        if len(raw) > 8 << 20:
            raise InspectionError('comparison_report_too_large')
        previous = json.loads(raw)
        if not isinstance(previous, dict):
            raise InspectionError('comparison_report_invalid')
        report['comparison'] = compare(previous, report)
    raw = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
    if args.output:
        fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'w') as handle:
            handle.write(raw)
            handle.flush()
            os.fsync(handle.fileno())
    print(raw, end='')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, TypeError, InspectionError) as error:
        print(json.dumps({'schema': SCHEMA, 'status': 'failed', 'reason': str(error) if isinstance(error, InspectionError) else type(error).__name__, 'authorizes_switch': False}))
        raise SystemExit(2)
