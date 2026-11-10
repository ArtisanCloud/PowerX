#!/usr/bin/env python3
"""T155 只读前置检查：只读取 Core 元数据，不测试云端、不接管、不授予执行许可。"""
import argparse
import datetime as dt
import ipaddress
import json
import os
from pathlib import Path
import re
import stat
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

MAX_RESPONSE = 1 << 20
MAX_TOKEN = 16 << 10
PATHS = ('/admin/ops/backup/archives/target-configuration',
         '/admin/ops/backup/archives/s3-credentials',
         '/admin/ops/backup/archives/policy-configuration')


class CheckError(Exception):
    """错误只携带固定代码，不回显令牌、原始响应或底层网络错误。"""


def is_loopback(host):
    if host and host.lower() == 'localhost':
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def core_url(value):
    try:
        u = urllib.parse.urlsplit(value)
        if (any(c.isspace() or ord(c) < 32 for c in value) or not u.hostname or
                u.username is not None or u.password is not None or u.query or u.fragment or
                u.scheme not in ('http', 'https') or
                (u.scheme == 'http' and not is_loopback(u.hostname)) or
                not re.fullmatch(r'(?:/[A-Za-z0-9_-]+)*/*', u.path)):
            raise ValueError()
        _ = u.port
    except (ValueError, TypeError):
        raise CheckError('CORE_URL_INVALID') from None
    return value.rstrip('/')


def token_value(raw):
    try:
        value = raw.decode('ascii').strip()
    except (UnicodeDecodeError, AttributeError):
        raise CheckError('SESSION_TOKEN_INVALID') from None
    if not value or len(raw) > MAX_TOKEN or any(c.isspace() or ord(c) < 33 or ord(c) > 126 for c in value):
        raise CheckError('SESSION_TOKEN_INVALID')
    return value


def read_token(path=None, source=None):
    if source is not None:
        return token_value(source.read(MAX_TOKEN + 1))
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as f:
            st = os.fstat(f.fileno())
            if not stat.S_ISREG(st.st_mode) or st.st_uid != os.geteuid() or st.st_nlink != 1 or st.st_mode & 0o077 or st.st_size > MAX_TOKEN:
                raise CheckError('SESSION_FILE_UNSAFE')
            return token_value(f.read(MAX_TOKEN + 1))
    except CheckError:
        raise
    except (OSError, ValueError, TypeError):
        raise CheckError('SESSION_FILE_UNAVAILABLE') from None


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise CheckError('RESPONSE_INVALID')
        result[key] = value
    return result


def fetch(base, path, token, timeout=8):
    # 不跟随重定向，不从环境启用代理；Authorization 只发送给显式 Core 地址。
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    req = urllib.request.Request(base + path, method='GET', headers={
        'Authorization': 'Bearer ' + token, 'Accept': 'application/json'})
    try:
        with opener.open(req, timeout=timeout) as response:
            raw = response.read(MAX_RESPONSE + 1)
            if len(raw) > MAX_RESPONSE:
                raise CheckError('RESPONSE_TOO_LARGE')
            value = json.loads(raw, object_pairs_hook=strict_object, parse_constant=lambda _: (_ for _ in ()).throw(ValueError()))
            data = value.get('data') if isinstance(value, dict) else None
            if not isinstance(data, dict):
                raise CheckError('RESPONSE_INVALID')
            return {'http_status': response.status, 'data': data}
    except urllib.error.HTTPError as e:
        code = e.code
        e.close()
        return {'http_status': code, 'problem': 'ROOT_SESSION_REQUIRED' if code in (401, 403) else 'HTTP_SOURCE_UNAVAILABLE'}
    except CheckError as e:
        return {'http_status': None, 'problem': str(e)}
    except (OSError, ValueError, UnicodeDecodeError, RecursionError):
        return {'http_status': None, 'problem': 'SOURCE_UNAVAILABLE'}


def positive(v):
    return type(v) is int and 0 < v <= 2**53 - 1


def control_ready(data):
    return (data.get('authority') == 'database' and data.get('ready') is True and
            data.get('state') == 'applied' and not data.get('problem') and
            positive(data.get('desired_revision')) and
            data['desired_revision'] == data.get('applied_revision') == data.get('snapshot_revision'))


def clean_text(v, maximum=256):
    return v if isinstance(v, str) and len(v) <= maximum and not any(ord(c) < 32 for c in v) else None


def endpoint_metadata(value):
    # 元数据也可能来自旧配置：含用户信息／query／fragment 的 URL 不写回报告。
    try:
        u = urllib.parse.urlsplit(value)
        if (not u.hostname or u.username is not None or u.password is not None or
                u.query or u.fragment or u.path not in ('', '/') or
                u.scheme not in ('https', 'http') or len(value) > 512):
            raise ValueError()
        _ = u.port
        return {'endpoint': u.scheme + '://' + u.netloc, 'loopback': is_loopback(u.hostname), 'https': u.scheme == 'https'}
    except (ValueError, TypeError):
        return {'problem': 'TARGET_ENDPOINT_INVALID'}


def evaluate(results, selected_id=None):
    issues = []
    tc, cv, pc = (r.get('data', {}) for r in results)
    observations = [{'path': path, 'http_status': r.get('http_status'), 'problem': r.get('problem')}
                    for path, r in zip(PATHS, results)]
    if any(r.get('http_status') != 200 or not isinstance(r.get('data'), dict) for r in results):
        issues.append('CORE_METADATA_UNAVAILABLE')
    target_control = {k: tc[k] for k in ('authority', 'ready', 'state', 'desired_revision', 'applied_revision', 'snapshot_revision') if k in tc}
    if not control_ready(tc):
        issues.append('TARGET_CONTROL_NOT_APPLIED')
    store = cv.get('store', {})
    if not isinstance(store, dict):
        store = {}
    store_safe = {k: store[k] for k in ('configured', 'ready', 'rotation_required', 'revision') if k in store}
    if store.get('configured') is not True or store.get('ready') is not True:
        issues.append('CREDENTIAL_STORE_NOT_READY')
    if store.get('rotation_required') is True:
        issues.append('CREDENTIAL_ROTATION_REQUIRED')
    targets = tc.get('targets', [])
    items = cv.get('items', [])
    if not isinstance(targets, list) or len(targets) > 128 or not isinstance(items, list) or len(items) > 128:
        targets, items = [], []
        issues.append('SOURCE_INVENTORY_INVALID')
    safe_targets = []
    selected = []
    for target in targets:
        if not isinstance(target, dict):
            issues.append('SOURCE_INVENTORY_INVALID')
            continue
        row = {k: clean_text(target.get(k)) for k in ('id', 'name', 'driver', 'bucket', 'prefix', 'region', 'credential_ref')}
        row.update(endpoint_metadata(target.get('endpoint')))
        row['revision'] = target.get('revision') if positive(target.get('revision')) else None
        row['verification_valid'] = target.get('verification_valid') is True
        row['force_path_style'] = target.get('force_path_style') if type(target.get('force_path_style')) is bool else None
        safe_targets.append(row)
        if target.get('id') == selected_id:
            selected.append(row)
    if not safe_targets:
        issues.append('TARGET_NOT_CONFIGURED')
    if selected_id is None:
        issues.append('TARGET_SELECTION_REQUIRED')
    elif len(selected) != 1:
        issues.append('TARGET_SELECTION_NOT_UNIQUE')
    else:
        row = selected[0]
        if row.get('driver') != 's3':
            issues.append('TARGET_S3_REQUIRED')
        if row.get('problem') or row.get('https') is not True or row.get('loopback') is True:
            issues.append('TARGET_LONG_TERM_ENDPOINT_UNCONFIRMED')
        if not row.get('bucket') or not row.get('prefix') or not row.get('region') or not row.get('revision'):
            issues.append('TARGET_SETTINGS_INCOMPLETE')
        creds = [c for c in items if isinstance(c, dict) and c.get('ref') == row.get('credential_ref')]
        if len(creds) != 1 or creds[0].get('origin') != 'sealed' or creds[0].get('state') != 'active' or not positive(creds[0].get('revision')):
            issues.append('TARGET_CREDENTIAL_NOT_ACTIVE')
        try:
            host = urllib.parse.urlsplit(row.get('endpoint', '')).hostname or ''
        except ValueError:
            host = ''
        if host.endswith('.aliyuncs.com'):
            if host.startswith('oss-'):
                issues.append('OSS_S3_ENDPOINT_REVIEW_REQUIRED')
            if row.get('force_path_style') is True:
                issues.append('OSS_VIRTUAL_HOST_REQUIRED')
        if row.get('verification_valid') is not True:
            issues.append('TARGET_NODE_VERIFICATION_REQUIRED')
    if not control_ready(pc):
        issues.append('POLICY_CONTROL_NOT_APPLIED')
    policy_safe = {k: pc[k] for k in ('authority', 'ready', 'state', 'desired_revision', 'applied_revision', 'snapshot_revision') if k in pc}
    policies = pc.get('policies', [])
    policy_safe['enabled_count'] = sum(isinstance(p, dict) and p.get('enabled') is True for p in policies) if isinstance(policies, list) else None
    return {'schema': 'powerx.archive-readiness/v1', 'observed_at': dt.datetime.now(dt.timezone.utc).isoformat(),
            'mode': 'read_only_preflight', 'selected_target_id': selected_id,
            'configuration_ready': not issues, 'issues': sorted(set(issues)),
            'target_control': target_control, 'credential_store': store_safe, 'policy_control': policy_safe,
            'targets': safe_targets, 'observations': observations,
            'independence_and_lifetime_verified': False, 'provider_protocol_verified': False,
            'transfer_performed': False, 'retrieval_performed': False, 't155_passed': False,
            'authorizes_configuration_change': False, 'authorizes_transfer': False,
            'required_live_evidence': ['independent_long_term_target', 'provider_protocol_and_versioning', 'private_bucket_and_lifecycle', 'owned_prefix_transfer_full_read', 'exact_retrieval_after_owned_source_loss', 'owned_object_cleanup_and_receipts']}


def write_report(path, report):
    # 只创建新私有报告；不覆盖历史回执或经软链跳转。
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    except OSError:
        raise CheckError('REPORT_CREATE_FAILED') from None
    with os.fdopen(fd, 'w') as f:
        json.dump(report, f, ensure_ascii=False, indent=2)
        f.write('\n')
        f.flush()
        os.fsync(f.fileno())


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--core-url', required=True, help='包含已配置 API 前缀；非本机必须 HTTPS')
    auth = parser.add_mutually_exclusive_group(required=True)
    auth.add_argument('--token-file', type=Path, help='当前用户私有普通单链接文件，不接收 S3 凭据')
    auth.add_argument('--token-stdin', action='store_true', help='从标准输入读取短期 Root 会话，不保存令牌')
    parser.add_argument('--target-id', help='显式选中的规范 UUID；不会默认选择首个目标')
    parser.add_argument('--output', type=Path, required=True, help='尚不存在的报告路径')
    args = parser.parse_args(argv)
    try:
        base = core_url(args.core_url)
        if args.target_id and (str(uuid.UUID(args.target_id)) != args.target_id or uuid.UUID(args.target_id).int == 0):
            raise CheckError('TARGET_ID_INVALID')
        token = read_token(args.token_file, sys.stdin.buffer if args.token_stdin else None)
        report = evaluate([fetch(base, path, token) for path in PATHS], args.target_id)
        if token in json.dumps(report, ensure_ascii=False):
            raise CheckError("SESSION_REFLECTED_IN_METADATA")
        write_report(args.output, report)
        print(json.dumps({'report_written': True, 'configuration_ready': report['configuration_ready'], 'issues': report['issues'], 't155_passed': False, 'authorizes_transfer': False}, ensure_ascii=False))
        return 0 if report['configuration_ready'] else 3
    except (CheckError, ValueError):
        print(json.dumps({'error': 'PREFLIGHT_INPUT_OR_OUTPUT_INVALID', 't155_passed': False, 'authorizes_transfer': False}), file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
