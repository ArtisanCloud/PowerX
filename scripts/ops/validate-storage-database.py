#!/usr/bin/env python3
"""在自有临时 PostgreSQL 中验收数据库期限和调用容量保护；不读取日常配置。"""

import argparse
import contextlib
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parents[2]


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    path.chmod(0o600)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pg-bin", type=Path, required=True, help="PostgreSQL bin 目录")
    parser.add_argument("--artifacts", type=Path, required=True, help="新的私有验收证据目录")
    args = parser.parse_args()
    args.pg_bin = args.pg_bin.resolve()
    os.umask(0o077)
    artifacts = args.artifacts.resolve()
    artifacts.mkdir(mode=0o700, parents=True, exist_ok=False)
    artifacts.chmod(0o700)
    for binary in ("initdb", "pg_ctl", "psql"):
        if not (args.pg_bin / binary).is_file():
            parser.error(f"缺少 {binary}")
    # 短 Unix socket 路径；不开放 TCP，不启动 Core/Nuxt，不使用外部 DSN。
    private = Path(tempfile.mkdtemp(prefix="px-sg-db-"))
    data = private / "data"
    port = 19432
    owner = {
        "owner": "codex-storage-database-validation",
        "cwd": str(ROOT),
        "data_directory": str(data),
        "socket_directory": str(private),
        "port": port,
        "tcp_enabled": False,
        "pid": None,
    }
    started = False
    results = []
    failure = None
    cleanup = {}
    processes = []

    def run(command, log, env=None, timeout=300):
        with log.open("wb") as output:
            process = subprocess.Popen(command, cwd=ROOT / "backend", env=env, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
            processes.append({"pid": process.pid, "command": command, "cwd": str(ROOT / "backend"), "ports": []})
            write_json(artifacts / "processes.json", processes)
            try:
                return process.wait(timeout=timeout)
            except BaseException:
                with contextlib.suppress(ProcessLookupError):
                    os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                raise

    def interrupted(signum, _frame):
        raise InterruptedError(f"signal {signum}")

    signal.signal(signal.SIGTERM, interrupted)
    try:
        command = [str(args.pg_bin / "initdb"), "-D", str(data), "-U", "storage_validation", "--auth-local=trust", "--auth-host=reject", "--no-locale", "-E", "UTF8"]
        if run(command, artifacts / "initdb.log"):
            raise RuntimeError("initdb failed; see private log")
        with (data / "postgresql.conf").open("a") as config:
            config.write("\nlisten_addresses=''\nlogging_collector=on\nlog_statement='mod'\nlog_directory='sql-log'\nlog_rotation_age='1h'\nlog_truncate_on_rotation=off\n")
        command = [str(args.pg_bin / "pg_ctl"), "-D", str(data), "-l", str(artifacts / "postgres.log"), "-o", f"-p {port} -k {private}", "-w", "-t", "20", "start"]
        owner["command"] = command
        write_json(artifacts / "ownership.json", owner)
        # 即便 start 的等待失败，也在 finally 根据私有 data 目录检查并停止。
        code = run(command, artifacts / "start.log")
        started = (data / "postmaster.pid").exists()
        if started:
            owner["pid"] = int((data / "postmaster.pid").read_text().splitlines()[0])
            write_json(artifacts / "ownership.json", owner)
        if code:
            raise RuntimeError("temporary PostgreSQL start failed")
        dsn = f"host={private} port={port} user=storage_validation dbname=postgres sslmode=disable"
        query = [str(args.pg_bin / "psql"), "-XAt", "--dbname", dsn, "-v", "ON_ERROR_STOP=1", "-c", "SELECT system_identifier::text FROM pg_control_system()"]
        cluster = subprocess.check_output(query, text=True, timeout=10).strip()
        write_json(artifacts / "validation.private.json", {"pg_dsn": dsn})
        env = dict(os.environ, POWERX_STORAGE_TEST_DSN=dsn, POWERX_STORAGE_TEST_CLUSTER_ID=cluster, POWERX_STORAGE_VALIDATION_CONFIG=str(artifacts / "validation.private.json"))
        commands = [
            ("api-admission", ["go", "test", "-race", "./internal/transport/http/admin/storage", "-run", "^TestDatabaseRetentionNativeAPIAndAdmission$", "-count=1", "-v"]),
            ("retention-repository", ["go", "test", "-race", "./pkg/corex/db/persistence/repository/capability_registry", "-run", "Retention|LegacyPending", "-count=1", "-v"]),
            ("grpc-capacity", ["go", "test", "-race", "./internal/transport/grpc/capability_registry", "-run", "^TestStorageCapacityRejectionPreservesGRPCContract$", "-count=1", "-v"]),
        ]
        for name, command in commands:
            began = time.monotonic()
            code = run(command, artifacts / f"{name}.log", env=env)
            results.append({"name": name, "command": command, "exit_code": code, "elapsed_seconds": round(time.monotonic() - began, 3)})
            print(json.dumps(results[-1]), flush=True)
        if any(item["exit_code"] for item in results):
            raise RuntimeError("acceptance failed; see private test logs")
    except (Exception, KeyboardInterrupt) as error:
        failure = str(error)
    finally:
        if started or (data / "postmaster.pid").exists():
            code = run([str(args.pg_bin / "pg_ctl"), "-D", str(data), "-m", "fast", "-w", "-t", "20", "stop"], artifacts / "stop.log", timeout=25)
            cleanup["stop_exit_code"] = code
        cleanup["postmaster_pid_file_removed"] = not (data / "postmaster.pid").exists()
        cleanup["socket_released"] = not (private / f".s.PGSQL.{port}").exists()
        if owner["pid"]:
            try:
                os.kill(owner["pid"], 0)
                cleanup["pid_exited"] = False
            except ProcessLookupError:
                cleanup["pid_exited"] = True
        else:
            cleanup["pid_exited"] = True
        sql_logs = list((data / "sql-log").glob("*"))
        # 不公开 SQL 正文；保留审计是否开启、DELETE 是否被记录和日志哈希。
        sql_evidence = []
        for log in sorted(sql_logs):
            content = log.read_bytes()
            sql_evidence.append({"name": log.name, "bytes": len(content), "sha256": hashlib.sha256(content).hexdigest(), "delete_recorded": b"DELETE FROM" in content})
        clean = all(cleanup[k] for k in ("postmaster_pid_file_removed", "socket_released", "pid_exited"))
        if clean:
            shutil.rmtree(private)
        else:
            failure = failure or "temporary PostgreSQL cleanup incomplete"
        cleanup["owned_cluster_removed"] = not private.exists()
        write_json(artifacts / "cleanup.json", cleanup)
        write_json(artifacts / "result.json", {"completed_at": dt.datetime.now(dt.timezone.utc).isoformat(), "passed": failure is None, "failure": failure, "tests": results, "sql_logging": {"collector": True, "statement": "mod", "logs": sql_evidence}, "cleanup": cleanup})
    print(json.dumps({"passed": failure is None, "artifacts": str(artifacts), "failure": failure, "cleanup": cleanup}, ensure_ascii=False), flush=True)
    return 0 if failure is None else 1


if __name__ == "__main__":
    raise SystemExit(main())
