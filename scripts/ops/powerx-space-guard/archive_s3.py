#!/usr/bin/env python3
"""Pull closed audit logs over SSH into a private, versioned S3 bucket.

Only a fresh full-object and manifest readback permits source expiry. No S3
credentials are copied to the source host. Offsite objects are never deleted.
"""
import argparse
import fcntl
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import socket
import signal
import subprocess
import sys
import tempfile
import time
import urllib.parse

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError
import yaml

from audit_custody import canonical, private_json
import guard


def build_client(settings):
    c = yaml.safe_load(Path(settings['s3_config_source']).read_text())['storage']['s3']
    endpoint = settings.get('endpoint') or c['endpoint']
    if '://' not in endpoint:
        endpoint = ('https://' if c.get('use_ssl', True) else 'http://') + endpoint
    u = urllib.parse.urlparse(endpoint)
    if u.username or u.password or u.query or u.fragment or u.path not in ['', '/'] or u.scheme not in ['https', 'http'] or not u.hostname:
        raise RuntimeError('s3_endpoint_invalid')
    if u.scheme != 'https' and u.hostname not in ['127.0.0.1', 'localhost', '::1']:
        raise RuntimeError('s3_tls_required')
    endpoint = endpoint.rstrip('/')
    client = boto3.client('s3', endpoint_url=endpoint, region_name=settings.get('region') or c.get('region') or 'us-east-1',
                          aws_access_key_id=c['access_key'], aws_secret_access_key=c['secret_key'],
                          aws_session_token=c.get('session_token') or None,
                          config=Config(signature_version='s3v4', connect_timeout=5, read_timeout=30,
                                        retries={'max_attempts': 2}, s3={'addressing_style': 'path' if c.get('force_path_style') else 'auto'}))
    fingerprint = hashlib.sha256((endpoint + '|' + settings['bucket']).encode()).hexdigest()
    return client, endpoint, fingerprint


def no_public_policy(policy):
    for statement in policy.get('Statement', []):
        if statement.get('Effect') != 'Allow':
            continue
        principal = statement.get('Principal')
        if principal is None or statement.get('NotPrincipal') is not None or principal == '*' or isinstance(principal, dict) and any(value == '*' or isinstance(value, list) and '*' in value for value in principal.values()):
            raise RuntimeError('audit_bucket_public_policy')


def require_private_versioned_bucket(client, bucket, initialize=False):
    try:
        client.head_bucket(Bucket=bucket)
    except ClientError as error:
        if not initialize or error.response['Error']['Code'] not in ['404', 'NoSuchBucket', 'NotFound']:
            raise RuntimeError('audit_bucket_unavailable') from None
        client.create_bucket(Bucket=bucket)
    try:
        no_public_policy(json.loads(client.get_bucket_policy(Bucket=bucket)['Policy']))
    except ClientError as error:
        if error.response['Error']['Code'] not in ['NoSuchBucketPolicy', 'NoSuchPolicy']:
            raise RuntimeError('audit_bucket_policy_unknown') from None
    acl = client.get_bucket_acl(Bucket=bucket)
    if any(grant.get('Grantee', {}).get('URI', '').rstrip('/').endswith(('/AllUsers', '/AuthenticatedUsers')) for grant in acl.get('Grants', [])):
        raise RuntimeError('audit_bucket_public_acl')
    try:
        lifecycle = client.get_bucket_lifecycle_configuration(Bucket=bucket)
        if any(rule.get('Status') == 'Enabled' and ('Expiration' in rule or 'NoncurrentVersionExpiration' in rule) for rule in lifecycle.get('Rules', [])):
            raise RuntimeError('audit_bucket_expiration_not_allowed')
    except ClientError as error:
        if error.response['Error']['Code'] not in ['NoSuchLifecycleConfiguration', 'NoSuchLifecycle']:
            raise RuntimeError('audit_bucket_lifecycle_unknown') from None
    if initialize:
        client.put_bucket_versioning(Bucket=bucket, VersioningConfiguration={'Status': 'Enabled'})
    if client.get_bucket_versioning(Bucket=bucket).get('Status') != 'Enabled':
        raise RuntimeError('audit_bucket_versioning_required')


def ssh_command(settings, operation):
    if operation not in ['manifest', 'read', 'confirm'] or not re.fullmatch('[A-Za-z0-9_.-]+', settings['ssh_host']):
        raise RuntimeError('ssh_binding_invalid')
    return ['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=10',
            '-l', settings.get('ssh_user', 'root'), settings['ssh_host'],
            '/usr/bin/python3', '/opt/powerx-ops/space-guard/audit_custody.py', operation]


def rpc(settings, operation, value=None):
    p = subprocess.run(ssh_command(settings, operation), input=canonical(value) if value is not None else None,
                       capture_output=True, timeout=45)
    if p.returncode or len(p.stdout) > 5 << 20:
        raise RuntimeError('custody_rpc_failed')
    return json.loads(p.stdout)


def full_readback(client, bucket, key, version, expected_hash, expected_size):
    if not version or version == 'null':
        raise RuntimeError('versioned_object_required')
    response = client.get_object(Bucket=bucket, Key=key, VersionId=version)
    body, digest, count = response['Body'], hashlib.sha256(), 0
    try:
        while True:
            part = body.read(1 << 20)
            if not part:
                break
            count += len(part)
            if count > expected_size:
                raise RuntimeError('archive_readback_size_mismatch')
            digest.update(part)
    finally:
        body.close()
    if count != expected_size or digest.hexdigest() != expected_hash or response.get('VersionId') != version:
        raise RuntimeError('archive_readback_digest_mismatch')


def download(settings, item, directory):
    free = os.statvfs(directory)
    if item['size'] > settings.get('max_file_bytes', 2 << 30) or free.f_bavail * free.f_frsize - item['size'] < settings.get('min_free_bytes', 20 << 30):
        raise RuntimeError('receiver_storage_insufficient')
    f = tempfile.NamedTemporaryFile(prefix='audit-download-', dir=directory, delete=False)
    path = Path(f.name)
    try:
        remaining = max(1, settings.get('_deadline', time.monotonic() + 180) - time.monotonic())
        p = subprocess.run(ssh_command(settings, 'read'), input=canonical(item), stdout=f, stderr=subprocess.PIPE, timeout=min(180, remaining))
        f.flush()
        os.fsync(f.fileno())
        f.close()
        if p.returncode or path.stat().st_size != item['size']:
            raise RuntimeError('source_transfer_failed')
        h = hashlib.sha256()
        with path.open('rb') as source:
            for block in iter(lambda: source.read(1 << 20), b''):
                h.update(block)
        return path, h.hexdigest()
    except BaseException:
        f.close()
        path.unlink(missing_ok=True)
        raise


def signed_receipt(settings, key, item, obj):
    proof = {'version': 1, 'receiver_id': settings['receiver_id'], 'verified_at': time.time(), 'snapshot': item, 'object': obj}
    return {'proof': proof, 'signature': hmac.new(key, canonical(proof), hashlib.sha256).hexdigest()}


def safe_reason(error):
    if isinstance(error, RuntimeError) and re.fullmatch('[a-z0-9_]+', str(error)):
        return str(error)
    if isinstance(error, ClientError):
        code = str(error.response.get('Error', {}).get('Code', 'unknown'))
        return 's3_' + code if re.fullmatch('[A-Za-z0-9_]+', code) else 's3_error'
    return type(error).__name__


def persist_manifest(client, settings, item, obj):
    key = settings['prefix'].rstrip('/') + '/manifests/' + item['id'] + '.json'
    data = canonical({'version': 1, 'receiver_id': settings['receiver_id'], 'verified_at': time.time(), 'snapshot': item, 'object': obj})
    digest = hashlib.sha256(data).hexdigest()
    response = client.put_object(Bucket=settings['bucket'], Key=key, Body=data, ContentType='application/json')
    version = response.get('VersionId')
    full_readback(client, settings['bucket'], key, version, digest, len(data))
    return {**obj, 'manifest_key': key, 'manifest_version_id': version, 'manifest_sha256': digest}


def run_once(settings):
    settings = dict(settings, _deadline=time.monotonic() + 240)
    directory = Path(settings['state_directory']).resolve()
    directory.mkdir(parents=True, mode=0o700, exist_ok=True)
    s = directory.stat()
    if s.st_uid != os.geteuid() or s.st_mode & 0o077:
        raise RuntimeError('receiver_state_unsafe')
    report = {'started_at': time.time(), 'status': 'running', 'uploaded_files': 0, 'uploaded_bytes': 0,
              'verified_files': 0, 'source_deleted': 0, 'source_retained': 0, 'source_commit_unconfirmed': 0, 'failed_files': 0, 'deferred_files': 0, 'offsite_objects_deleted': 0, 'failure_reasons': {}}
    with (directory / 'receiver.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        try:
            inventory = rpc(settings, 'manifest')
            if inventory.get('archive_enabled') is not True:
                report['status'] = 'disabled'
                report['control_state'] = 'not_enabled_or_managed_by_powerx'
                return report
            key_info = private_json(Path(settings['receipt_key_file']))
            secret = bytes.fromhex(key_info['key_hex'])
            if len(secret) != 32:
                raise RuntimeError('receiver_key_invalid')
            client, _, endpoint_id = build_client(settings)
            require_private_versioned_bucket(client, settings['bucket'])
            if inventory.get('version') != 1 or not inventory.get('complete') or len(inventory['files']) > 4096:
                raise RuntimeError('inventory_incomplete')
            eligible_ids = set(inventory['eligible_ids']) if 'eligible_ids' in inventory else None
            index_path = directory / 'objects.json'
            index = json.loads(index_path.read_text()) if index_path.exists() else {}
            # 源文件消失不能抹去历史对象引用；正式历史目录由 Core 纳管。
            for item in inventory['files']:
                if time.monotonic() >= settings['_deadline']:
                    report['status'] = 'partial'
                    break
                path = None
                try:
                    obj = index.get(item['id'])
                    if obj and (obj.get('endpoint_id') != endpoint_id or obj.get('bucket') != settings['bucket']):
                        obj = None
                    if obj:
                        try:
                            head = client.head_object(Bucket=settings['bucket'], Key=obj['key'], VersionId=obj['version_id'])
                            if head['ContentLength'] != item['size']:
                                obj = None
                        except ClientError:
                            obj = None
                    fresh = obj is None
                    if fresh:
                        if report['uploaded_files'] >= settings.get('max_files_per_run', 128) or report['uploaded_bytes'] + item['size'] > settings.get('max_bytes_per_run', 1 << 30):
                            report['status'] = 'partial'
                            break
                        path, digest = download(settings, item, directory)
                        object_key = settings['prefix'].rstrip('/') + '/objects/' + digest[:2] + '/' + digest
                        with path.open('rb') as data:
                            response = client.put_object(Bucket=settings['bucket'], Key=object_key, Body=data,
                                                         ContentLength=item['size'], ContentType='application/octet-stream', Metadata={'sha256': digest})
                        obj = {'endpoint_id': endpoint_id, 'bucket': settings['bucket'], 'key': object_key,
                               'version_id': response.get('VersionId'), 'sha256': digest, 'bytes': item['size']}
                        report['uploaded_files'] += 1
                        report['uploaded_bytes'] += item['size']
                    # The source independently checks seven-day age and the
                    # approved budget policy (which always keeps the latest day).
                    expired = item['id'] in eligible_ids if eligible_ids is not None else item['mtime_ns'] / 1e9 < time.time() - 7 * 86400
                    if fresh or expired:
                        full_readback(client, settings['bucket'], obj['key'], obj['version_id'], obj['sha256'], item['size'])
                        proof_object = persist_manifest(client, settings, item, obj)
                        report['verified_files'] += 1
                        # Sign only after both versioned objects have been read in full.
                        receipt = signed_receipt(settings, secret, item, proof_object)
                        if expired:
                            try:
                                result = rpc(settings, 'confirm', receipt)
                            except Exception:
                                report['source_commit_unconfirmed'] += 1
                                raise RuntimeError('source_commit_unconfirmed') from None
                            if result['state'] == 'deleted':
                                report['source_deleted'] += 1
                            elif result['state'] == 'retained':
                                report['source_retained'] += 1
                            else:
                                raise RuntimeError('source_commit_blocked')
                    index[item['id']] = obj
                    guard.atomic_json(index_path, index)
                except Exception as error:
                    if isinstance(error, subprocess.TimeoutExpired) and time.monotonic() >= settings['_deadline'] - .5:
                        report['status'] = 'partial'
                        report['deferred_files'] += 1
                        break
                    report['failed_files'] += 1
                    reason = safe_reason(error)
                    report['failure_reasons'][reason] = report['failure_reasons'].get(reason, 0) + 1
                finally:
                    if path is not None:
                        path.unlink(missing_ok=True)
            report['status'] = 'failed' if report['failed_files'] else 'ok' if report['status'] == 'running' else report['status']
        except Exception as error:
            report['status'] = 'failed'
            report['reason'] = safe_reason(error)
        finally:
            report['finished_at'] = time.time()
            guard.atomic_json(directory / 'last-run.json', report)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', required=True)
    parser.add_argument('--initialize', action='store_true')
    args = parser.parse_args()
    os.umask(0o077)
    def timed_out(_signal, _frame):
        raise TimeoutError('archive_run_deadline')
    signal.signal(signal.SIGALRM, timed_out)
    signal.alarm(300)
    try:
        settings = private_json(Path(args.config))
        if args.initialize:
            client, _, _ = build_client(settings)
            require_private_versioned_bucket(client, settings['bucket'], initialize=True)
            report = {'status': 'initialized', 'private_policy_checked': True, 'versioning': 'Enabled', 'automatic_expiration': False}
        else:
            report = run_once(settings)
        print(json.dumps(report))
        return 0 if report['status'] in ['ok', 'initialized', 'disabled'] else 2
    except Exception:
        print(json.dumps({'status': 'failed', 'reason': 'archive_configuration_or_transport_unavailable'}))
        return 2


if __name__ == '__main__':
    sys.exit(main())
