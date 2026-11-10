#!/usr/bin/env python3
"""用新建 PostgreSQL / MinIO 验收真实归档取回和数据库备份恢复，不读取日常配置。"""

import argparse
import contextlib
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request


ROOT = Path(__file__).resolve().parents[2]


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    path.chmod(0o600)


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def listening(port):
    with socket.socket() as sock:
        sock.settimeout(0.2)
        return sock.connect_ex(("127.0.0.1", port)) == 0


def pid_exists(pid):
    if pid is None:
        return False
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pg-bin", type=Path, required=True)
    parser.add_argument("--minio-bin", default=shutil.which("minio"))
    parser.add_argument("--artifacts", type=Path, required=True, help="新的私有证据目录")
    args = parser.parse_args()
    pg = args.pg_bin.resolve()
    for name in ("initdb", "pg_ctl", "psql", "pg_dump", "pg_restore", "createdb", "dropdb"):
        if not (pg / name).is_file():
            parser.error(f"缺少 PostgreSQL 工具：{name}")
    if not args.minio_bin or not shutil.which(args.minio_bin):
        parser.error("需要现有 MinIO 可执行文件；脚本不下载或替换软件")
    # 只用于本脚本新建的回环服务。凭据随机产生，不接收现有远端配置。
    import boto3
    from botocore.config import Config

    os.umask(0o077)
    out = args.artifacts.resolve()
    out.mkdir(parents=True, mode=0o700, exist_ok=False)
    private = Path(tempfile.mkdtemp(prefix="powerx-control-owned-artifacts-", dir="/tmp"))
    data = private / "postgres"
    pg_port, s3_port, console_port = free_port(), free_port(), free_port()
    while len({pg_port, s3_port, console_port}) != 3:
        pg_port, s3_port, console_port = free_port(), free_port(), free_port()
    cfg = {"endpoint": f"http://127.0.0.1:{s3_port}", "bucket": "powerx-storage-validation", "region": "us-east-1", "access_key": "pxowned" + secrets.token_hex(8), "secret_key": secrets.token_urlsafe(36)}
    secret_path = private / "s3.private.json"
    save(secret_path, cfg)
    owners, processes, handles, results = [], [], [], []
    pg_pid, minio, failure, pg_stopped = None, None, None, False
    sql_audit, inventory, remote = {}, {}, {}

    def start(command, name, env=None, ports=()):
        handle = (out / name).open("wb")
        handles.append(handle)
        proc = subprocess.Popen(command, cwd=ROOT / "backend", env=env, stdout=handle, stderr=subprocess.STDOUT, start_new_session=True)
        processes.append(proc)
        owners.append({"pid": proc.pid, "command": command, "cwd": str(ROOT / "backend"), "ports": list(ports)})
        save(out / "processes.json", owners)
        return proc

    def run(command, name, env=None, timeout=240):
        proc = start(command, name, env)
        return proc.wait(timeout=timeout)

    def stop(proc):
        if proc.poll() is None:
            with contextlib.suppress(ProcessLookupError):
                os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                with contextlib.suppress(ProcessLookupError):
                    os.killpg(proc.pid, signal.SIGKILL)
                proc.wait(timeout=10)

    def interrupted(signum, _frame):
        raise InterruptedError(f"signal {signum}")

    def query(sql):
        # 连接仅指向本脚本拥有的 Unix socket；不使用 PG* 或日常配置。
        return subprocess.check_output([str(pg / "psql"), "-XAt", "--dbname", dsn, "-v", "ON_ERROR_STOP=1", "-c", sql], text=True, timeout=10).strip()

    # 清除可能将工具重定向到日常环境、改变测试含义的继承变量。
    env = {k: v for k, v in os.environ.items() if not k.startswith(("PG", "POWERX_", "MINIO_"))}
    env["PATH"] = str(pg) + os.pathsep + env.get("PATH", "")
    env["GOWORK"] = "off"
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        init = [str(pg / "initdb"), "-D", str(data), "-U", "storage_control_test", "--auth-local=trust", "--auth-host=reject", "--no-locale", "-E", "UTF8"]
        if run(init, "initdb.log", env):
            raise RuntimeError("initdb failed; see private log")
        with (data / "postgresql.conf").open("a") as f:
            f.write("\nlisten_addresses=''\nlogging_collector=on\nlog_statement='mod'\nlog_directory='sql-log'\nlog_truncate_on_rotation=off\n")
        command = [str(pg / "pg_ctl"), "-D", str(data), "-l", str(out / "postgres.log"), "-o", f"-p {pg_port} -k {private}", "-w", "-t", "20", "start"]
        code = run(command, "postgres-start.log", env)
        if (data / "postmaster.pid").exists():
            pg_pid = int((data / "postmaster.pid").read_text().splitlines()[0])
            owners.append({"pid": pg_pid, "command": command, "cwd": str(ROOT / "backend"), "port": pg_port, "tcp": False, "socket": str(private), "data_directory": str(data)})
            save(out / "processes.json", owners)
        if code:
            raise RuntimeError("owned PostgreSQL startup failed")
        dsn = "postgresql://storage_control_test@/postgres?" + urllib.parse.urlencode({"host": str(private), "port": pg_port, "sslmode": "disable"})
        minio_env = dict(env, MINIO_ROOT_USER=cfg["access_key"], MINIO_ROOT_PASSWORD=cfg["secret_key"], MINIO_REGION_NAME=cfg["region"], MINIO_SITE_REGION=cfg["region"])
        minio = start([str(Path(args.minio_bin).resolve()), "--certs-dir", str(private / "certs"), "server", str(private / "minio"), "--address", f"127.0.0.1:{s3_port}", "--console-address", f"127.0.0.1:{console_port}"], "minio.log", minio_env, (s3_port, console_port))
        deadline = time.monotonic() + 25
        while True:
            try:
                with urllib.request.urlopen(cfg["endpoint"] + "/minio/health/live", timeout=1) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            if minio.poll() is not None or time.monotonic() > deadline:
                raise RuntimeError("owned MinIO startup failed")
            time.sleep(0.2)
        client = boto3.client("s3", endpoint_url=cfg["endpoint"], region_name=cfg["region"], aws_access_key_id=cfg["access_key"], aws_secret_access_key=cfg["secret_key"], config=Config(s3={"addressing_style": "path"}, retries={"max_attempts": 1}))
        client.create_bucket(Bucket=cfg["bucket"])
        client.put_bucket_versioning(Bucket=cfg["bucket"], VersioningConfiguration={"Status": "Enabled"})
        location = client.get_bucket_location(Bucket=cfg["bucket"]).get("LocationConstraint") or "us-east-1"
        if location != cfg["region"]:
            raise RuntimeError("owned MinIO region does not match explicit test configuration")
        print(json.dumps({"owned_services_ready": True, "postgres_pid": pg_pid, "minio_pid": minio.pid, "s3_endpoint": cfg["endpoint"]}), flush=True)
        env.update(POWERX_STORAGE_NATIVE_ARTIFACTS="1", POWERX_STORAGE_CONTROL_TEST_DSN=dsn, POWERX_STORAGE_VALIDATION_CONFIG=str(secret_path), POWERX_ARCHIVE_TEST_DSN=dsn, POWERX_BACKUP_TEST_ADMIN_DSN=dsn)
        suites = [
            ("governed-artifacts", "./internal/service/storage_governance", ["TestStorageNativeRealBackupRestore", "TestStorageNativeRealS3ArchiveRecovery", "TestLifecycleNativePostgresAdoptionPublicationDeletionAndReceipts"]),
            ("archive-native", "./internal/service/archive_ops", ["TestLiveS3Governance", "TestMediaDatabaseSnapshotNativePostgres"]),
            ("backup-native", "./internal/service/backup_ops", ["TestBackupIntegration"]),
        ]
        for name, package, expected in suites:
            command = ["go", "test", "-json", "-race", package, "-run", "^(" + "|".join(expected) + ")$", "-count=1", "-parallel=1", "-timeout=180s"]
            began = time.monotonic()
            code = run(command, name + ".jsonl", env)
            final = {}
            for line in (out / (name + ".jsonl")).read_text().splitlines():
                with contextlib.suppress(json.JSONDecodeError):
                    event = json.loads(line)
                    if event.get("Test") in expected and event.get("Action") in ("pass", "fail", "skip"):
                        final[event["Test"]] = event["Action"]
            passed = code == 0 and all(final.get(test) == "pass" for test in expected)
            results.append({"name": name, "command": command, "exit_code": code, "tests": final, "passed": passed, "elapsed_seconds": round(time.monotonic() - began, 3)})
            save(out / "tests.json", results)
            print(json.dumps({"suite": name, "passed": passed, "tests": final}), flush=True)
            if not passed:
                raise RuntimeError(f"{name} failed or skipped; see private JSONL")
        sql_audit = json.loads(query("SELECT json_build_object('logging_collector',current_setting('logging_collector'),'log_statement',current_setting('log_statement'),'log_truncate_on_rotation',current_setting('log_truncate_on_rotation'))"))
        inventory = json.loads(query("SELECT json_build_object('databases',(SELECT json_agg(datname ORDER BY datname) FROM pg_database WHERE NOT datistemplate),'owned_schemas',(SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'control_owned_%'))"))
        if inventory["databases"] != ["postgres"] or inventory["owned_schemas"] != 0:
            raise RuntimeError("test database/schema cleanup incomplete")
        if sql_audit != {"logging_collector": "on", "log_statement": "mod", "log_truncate_on_rotation": "off"}:
            raise RuntimeError("SQL modification logging changed unexpectedly")
        versions = []
        for page in client.get_paginator("list_object_versions").paginate(Bucket=cfg["bucket"]):
            versions.extend(page.get("Versions", []))
        remote = {"versioning": client.get_bucket_versioning(Bucket=cfg["bucket"]).get("Status"), "versions": len(versions), "bytes": sum(v["Size"] for v in versions), "scope": "owned ephemeral loopback MinIO; not a cloud or long-lived target"}
    except (Exception, KeyboardInterrupt) as error:
        # 不回显 SDK 连接信息和凭据；细节仅由测试安全日志记录。
        failure = type(error).__name__ + ": " + str(error)
        for secret in (cfg["access_key"], cfg["secret_key"]):
            failure = failure.replace(secret, "[redacted]")
    finally:
        cleanup_errors = []
        for proc in reversed(processes):
            try:
                stop(proc)
            except Exception as error:
                cleanup_errors.append({"pid": proc.pid, "error": type(error).__name__})
        if (data / "postmaster.pid").exists():
            # 核实完整私有 data 路径，由 pg_ctl 停止本轮拥有的集群及其子进程。
            try:
                with (out / "postgres-stop.log").open("wb") as log:
                    result = subprocess.run([str(pg / "pg_ctl"), "-D", str(data), "-m", "fast", "-w", "-t", "20", "stop"], stdout=log, stderr=subprocess.STDOUT, timeout=25)
                    pg_stopped = result.returncode == 0
            except Exception as error:
                cleanup_errors.append({"pid": pg_pid, "error": type(error).__name__})
        else:
            pg_stopped = True
        for handle in handles:
            handle.close()
        cleanup = {"recorded_processes_exited": all(proc.poll() is not None for proc in processes), "postgres_stopped": pg_stopped and not (data / "postmaster.pid").exists(), "postgres_pid_exited": not pid_exists(pg_pid), "postgres_socket_released": not (private / f".s.PGSQL.{pg_port}").exists(), "minio_ports_released": not listening(s3_port) and not listening(console_port)}
        if all(cleanup.values()) and not cleanup_errors:
            try:
                shutil.rmtree(private)
            except OSError as error:
                cleanup_errors.append({"error": type(error).__name__, "operation": "owned_data_removal"})
        cleanup["owned_data_and_credentials_removed"] = not private.exists()
        # 日志也不得意外保留测试凭据。
        for path in list(out.glob("*.log")) + list(out.glob("*.jsonl")):
            value = path.read_text(errors="replace")
            redacted = value.replace(cfg["access_key"], "[redacted]").replace(cfg["secret_key"], "[redacted]")
            if value != redacted:
                path.write_text(redacted)
        if not all(cleanup.values()) or cleanup_errors:
            failure = failure or "owned service cleanup incomplete"
        source_files = ["scripts/ops/validate-storage-artifacts.py", "backend/internal/service/storage_governance/recovery_artifacts_postgres_test.go", "backend/scripts/ops/backup-db.sh", "backend/scripts/ops/restore-drill.sh", "backend/internal/service/archive_ops/live_s3_test.go", "backend/internal/service/archive_ops/postgres_test.go", "backend/internal/service/backup_ops/integration_test.go", "backend/internal/service/storage_governance/lifecycle_control_postgres_test.go"]
        report = {"checked_at": dt.datetime.now().astimezone().isoformat(), "passed": failure is None, "failure": failure, "scope": "source and owned native artifact acceptance only", "tests": results, "sql_audit": sql_audit, "remaining_database_inventory": inventory, "remote": remote, "cleanup": cleanup, "cleanup_errors": cleanup_errors, "not_performed": ["daily/production migration, restart, policy adoption or data deletion", "long-lived remote target validation", "whole-site physical restore or actual customer-data restore", "external executor handoff", "7-day/14-day runtime acceptance"], "source_sha256": {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in source_files}}
        save(out / "result.json", report)
        print(json.dumps({"passed": report["passed"], "failure": failure, "cleanup": cleanup, "report": str(out / "result.json")}), flush=True)
    return 0 if failure is None else 1


if __name__ == "__main__":
    raise SystemExit(main())
