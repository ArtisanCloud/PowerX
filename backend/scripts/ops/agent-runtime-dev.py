#!/usr/bin/env python3
"""本机 Agent Runtime 依赖启动与预检；凭据只从私有配置读取，不输出。"""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import time
from urllib.parse import urlparse
import uuid

import yaml

ROOT = Path(__file__).resolve().parents[2]


def config(path):
    value = yaml.safe_load(Path(path).read_text())
    if value.get('deployment', {}).get('env') != 'dev':
        raise RuntimeError('此命令仅支持 deployment.env=dev')
    return value


def s3_client(c):
    import boto3
    from botocore.config import Config
    s = c['storage']['s3']
    return boto3.client('s3', endpoint_url=s['endpoint'],
                       aws_access_key_id=s['access_key'], aws_secret_access_key=s['secret_key'],
                       aws_session_token=s.get('session_token') or None,
                       region_name=s.get('region') or 'us-east-1',
                       config=Config(connect_timeout=5, read_timeout=10,
                                     retries={'max_attempts': 1},
                                     s3={'addressing_style': 'path' if s.get('force_path_style') else 'auto'}))


def minio(c, data):
    s = c['storage']['s3']
    url = urlparse(s['endpoint'])
    if url.scheme != 'http' or url.hostname not in ('127.0.0.1', 'localhost') or url.port != 9000:
        raise RuntimeError('MinIO 启动仅支持本机 http://127.0.0.1:9000 配置')
    if s.get('session_token'):
        raise RuntimeError('本机 MinIO 根凭据不支持 session_token')
    executable = shutil.which('minio')
    if not executable:
        raise RuntimeError('请先安装 minio')
    with socket.socket() as sock:
        if sock.connect_ex(('127.0.0.1', 9000)) == 0:
            raise RuntimeError('9000 已有服务；请使用 check 验证现有实例')
    directory = Path(data).resolve()
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    env = os.environ.copy()
    env['MINIO_ROOT_USER'], env['MINIO_ROOT_PASSWORD'] = s['access_key'], s['secret_key']
    env['MINIO_BROWSER'] = 'off'
    for name in ('MINIO_REGION', 'MINIO_REGION_NAME', 'MINIO_SITE_REGION'):
        env[name] = s.get('region') or 'us-east-1'
    env['MINIO_CONFIG_ENV_FILE'] = '/dev/null'
    # MinIO 启动输出可能包含管理信息；日志保持私有，不回显到终端。
    log = directory / 'server.log'
    fd = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    child = subprocess.Popen([executable, 'server', str(directory / 'objects'),
                              '--address', '127.0.0.1:9000', '--console-address', '127.0.0.1:9001'],
                             env=env, stdout=fd, stderr=subprocess.STDOUT)
    os.fchmod(fd, 0o600)
    os.close(fd)
    def stop(_signum, _frame):
        if child.poll() is None:
            child.terminate()
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    print(f'MinIO pid={child.pid}，数据目录={directory}；Ctrl+C 停止，数据保留', flush=True)
    try:
        return child.wait()
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=15)


def check(c, create):
    fabric = c['event']['fabric']
    address = fabric['redis_addr']
    host, port = address.rsplit(':', 1)
    env = os.environ.copy()
    password = fabric.get('redis_password')
    if password:
        env['REDISCLI_AUTH'] = password
    def redis(*args):
        return subprocess.check_output(['redis-cli', '-h', host, '-p', port, '--raw', *args],
                                       env=env, text=True, timeout=10).strip()
    for name, expected in [('appendonly', 'yes'), ('maxmemory-policy', 'noeviction')]:
        if redis('CONFIG', 'GET', name).splitlines()[-1] != expected:
            raise RuntimeError(f'Redis {name} 必须为 {expected}')
    persistence = dict(line.split(':', 1) for line in redis('INFO', 'persistence').splitlines()
                       if ':' in line and not line.startswith('#'))
    if persistence.get('aof_enabled') != '1' or persistence.get('aof_last_write_status') != 'ok' or persistence.get('aof_rewrite_in_progress') != '0':
        raise RuntimeError('Redis AOF 尚未就绪或写入异常')
    print('Redis AOF/noeviction/写入状态通过；appendfsync=' + redis('CONFIG', 'GET', 'appendfsync').splitlines()[-1])
    client = s3_client(c)
    bucket = c.get('ai', {}).get('runtime', {}).get('durable_sessions', {}).get('report_bucket') or 'powerx-agent-runs'
    from botocore.exceptions import ClientError
    try:
        client.head_bucket(Bucket=bucket)
    except ClientError as exc:
        if not create or exc.response['ResponseMetadata']['HTTPStatusCode'] != 404:
            raise RuntimeError('报告桶不可访问；检查桶和凭据权限') from None
        client.create_bucket(Bucket=bucket)
    actual_region = client.get_bucket_location(Bucket=bucket).get('LocationConstraint') or 'us-east-1'
    if actual_region != (c['storage']['s3'].get('region') or 'us-east-1'):
        raise RuntimeError('MinIO/S3 区域与 Core 配置不一致')
    # 不调用 put_bucket_policy；新建桶默认私有。
    key = 'agent-runtime/probes/dev-' + str(uuid.uuid4())
    payload = b'{"schema":"powerx.agent.storage-probe/v1"}'
    try:
        client.put_object(Bucket=bucket, Key=key, Body=payload, ContentType='application/json')
        body = client.get_object(Bucket=bucket, Key=key)['Body']
        try:
            actual = body.read()
        finally:
            body.close()
        if hashlib.sha256(actual).digest() != hashlib.sha256(payload).digest():
            raise RuntimeError('对象存储读回校验失败')
    finally:
        client.delete_object(Bucket=bucket, Key=key)
    print(f'S3 桶 {bucket} 写入、读回校验、删除通过')


def prepare(c, dest, port):
    if Path(dest).resolve() == (ROOT / 'etc/config.yaml').resolve():
        raise RuntimeError('请指定独立联调配置；正式配置启用需在重启节点处理')
    r = c.setdefault('ai', {}).setdefault('runtime', {})
    r['durable_sessions'] = dict(enabled=True, admin_chat_enabled=True, report_bucket='powerx-agent-runs',
                                 worker_concurrency=4, scan_interval='5s', lease_ttl='30s')
    r['run_deadline'], r['queue_wait_timeout'] = '30m', '10m'
    r['physical_model_pools'] = [dict(pool_id='ollama-local-qwen3-8b', provider='ollama',
                                    endpoint='http://127.0.0.1:11434', model='qwen3:8b',
                                    capacity=1, max_waiting=100, lease_ttl='6m')]
    c['server']['host'] = '127.0.0.1'
    c['server']['port'] = port
    c['server'].setdefault('grpc', {})['host'] = '127.0.0.1'
    c['server'].setdefault('grpc', {})['port'] = port + 1
    c['agent']['port'] = port + 2
    c.setdefault('log', {}).setdefault('file', {})['enable'] = False
    target = Path(dest).resolve()
    target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    os.fchmod(fd, 0o600)
    with os.fdopen(fd, 'w') as stream:
        yaml.safe_dump(c, stream, allow_unicode=True, sort_keys=False)
    print(f'联调配置已写入 {target}，HTTP={port}，GRPC={port+1}；使用 POWERX_CONFIG 指定')


def core(c, path, binary):
    durable = c.get('ai', {}).get('runtime', {}).get('durable_sessions', {})
    if not durable.get('enabled') or not durable.get('admin_chat_enabled'):
        raise RuntimeError('联调配置必须启用 durable_sessions 和 admin_chat_enabled')
    check(c, False)
    port = c['server']['port']
    grpc_port = c['server']['grpc']['port']
    if port == 8077:
        raise RuntimeError('联调命令要求独立 HTTP 端口；正式后台重启使用原启动方式')
    for value in (port, grpc_port):
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1', value)) == 0:
                raise RuntimeError(f'端口 {value} 已被占用')
    executable = str(Path(binary).resolve())
    if not Path(executable).is_file():
        raise RuntimeError('请先在 backend 执行 go build -o 指定文件 ./cmd/app')
    env = os.environ.copy()
    env.update(POWERX_CONFIG=str(Path(path).resolve()), POWERX_BACKEND_PORT=str(port),
               POWERX_GRPC_PORT=str(grpc_port), CORE_X_LOG_CONSOLE='true', CORE_X_LOG_FILE_ENABLE='false')
    log = Path(path).resolve().parent / 'server.log'
    fd = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    child = subprocess.Popen([executable], cwd=ROOT, env=env, stdout=fd, stderr=subprocess.STDOUT)
    os.fchmod(fd, 0o600)
    os.close(fd)
    def stop(_signum, _frame):
        if child.poll() is None:
            child.terminate()
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    print(f'联调 Core pid={child.pid}，HTTP={port}，日志={log}；Ctrl+C 停止', flush=True)
    try:
        from urllib.request import urlopen
        ready = False
        for _ in range(60):
            if child.poll() is not None:
                raise RuntimeError('Core 在健康检查前退出；查看私有启动日志')
            try:
                with urlopen(f'http://127.0.0.1:{port}/api/v1/health', timeout=1) as response:
                    ready = response.status == 200
            except Exception:
                pass
            if ready:
                break
            time.sleep(1)
        if not ready:
            raise RuntimeError('Core 启动健康检查超时；查看私有启动日志')
        print('持久化 Core 启动和 HTTP 健康检查通过', flush=True)
        return child.wait()
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=20)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', default=str(ROOT / 'etc/config.yaml'))
    commands = parser.add_subparsers(dest='command', required=True)
    m = commands.add_parser('minio')
    m.add_argument('--data-dir', default=str(ROOT / '.cache/agent-runtime/minio'))
    p = commands.add_parser('prepare-config')
    p.add_argument('--output', required=True)
    p.add_argument('--port', type=int, default=18077)
    server = commands.add_parser('core')
    server.add_argument('--binary', required=True)
    k = commands.add_parser('check')
    k.add_argument('--create-bucket', action='store_true')
    args = parser.parse_args()
    c = config(args.config)
    if args.command == 'minio':
        return minio(c, args.data_dir)
    if args.command == 'core':
        return core(c, args.config, args.binary)
    if args.command == 'prepare-config':
        prepare(c, args.output, args.port)
    else:
        check(c, args.create_bucket)
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as exc:
        # 不打印客户端异常内容：可能带原始请求或凭据。
        print('操作失败：' + (str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
