"""Run dataset governance acceptance on two privately owned PostgreSQL clusters and Redis."""

import argparse
from contextlib import contextmanager, nullcontext
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import time
import urllib.request
from tempfile import TemporaryDirectory


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def stop_owned(process):
    if process is not None and process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


@contextmanager
def minio_fixture(fixture):
    import boto3
    from botocore.config import Config

    port, console = free_port(), free_port()
    while console == port:
        console = free_port()
    config = {
        "endpoint": f"http://127.0.0.1:{port}", "bucket": "powerx-dataset-validation",
        "region": "us-east-1", "access_key": secrets.token_hex(12), "secret_key": secrets.token_urlsafe(36),
    }
    private = fixture / "s3.json"
    private.write_text(json.dumps(config))
    private.chmod(0o600)
    env = {key: value for key, value in os.environ.items() if not key.startswith("MINIO_")}
    env.update(MINIO_ROOT_USER=config["access_key"], MINIO_ROOT_PASSWORD=config["secret_key"],
               MINIO_REGION_NAME=config["region"], MINIO_SITE_REGION=config["region"], MINIO_UPDATE="off")
    command = ["minio", "--certs-dir", str(fixture / "certs"), "server", str(fixture / "minio"),
               "--address", f"127.0.0.1:{port}", "--console-address", f"127.0.0.1:{console}"]
    with (fixture / "minio.log").open("w") as log:
        process = subprocess.Popen(command, cwd=fixture, env=env, stdout=log, stderr=log)
    print(f"Owned MinIO pid={process.pid} cwd={fixture} ports={[port, console]} cmd={command}", flush=True)
    try:
        for _ in range(80):
            try:
                with urllib.request.urlopen(config["endpoint"] + "/minio/health/live", timeout=1):
                    break
            except OSError:
                if process.poll() is not None:
                    raise RuntimeError("owned MinIO exited during startup")
                time.sleep(0.2)
        else:
            raise RuntimeError("owned MinIO did not become ready")
        client = boto3.client("s3", endpoint_url=config["endpoint"], region_name=config["region"],
                              aws_access_key_id=config["access_key"], aws_secret_access_key=config["secret_key"],
                              config=Config(s3={"addressing_style": "path"}))
        client.create_bucket(Bucket=config["bucket"])
        client.put_bucket_versioning(Bucket=config["bucket"], VersioningConfiguration={"Status": "Enabled"})
        location = client.get_bucket_location(Bucket=config["bucket"]).get("LocationConstraint") or "us-east-1"
        if location != config["region"] or client.get_bucket_versioning(Bucket=config["bucket"]).get("Status") != "Enabled":
            raise RuntimeError("owned MinIO bucket does not match the versioning and region contract")
        client.close()
        yield str(private)
    finally:
        stop_owned(process)
        for target in (port, console):
            with socket.socket() as listener:
                if listener.connect_ex(("127.0.0.1", target)) == 0:
                    raise RuntimeError(f"owned MinIO port {target} remained open")
        print("Owned MinIO stopped and ports released.", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--with-minio", action="store_true", help="verify the full archive path on owned MinIO; requires boto3")
    options = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    bin_dir = os.environ.get("POWERX_STORAGE_TEST_PG_BIN", "")
    def pg(name):
        return str(Path(bin_dir) / name) if bin_dir else shutil.which(name) or name

    with TemporaryDirectory(prefix="powerx-control-owned-", dir="/tmp") as private_dir:
        fixture = Path(private_dir)
        clusters = [(fixture / "data", "19443"), (fixture / "plugin-data", "19446")]
        redis_process = None
        redis_socket = fixture / "redis.sock"
        try:
            for cluster, port in clusters:
                subprocess.run(
                    [pg("initdb"), "-D", str(cluster), "-U", "storage_control_test", "-A", "trust",
                     "--encoding=UTF8", "--no-locale"],
                    check=True, stdout=subprocess.DEVNULL,
                )
                start = [pg("pg_ctl"), "-D", str(cluster), "-w", "-l",
                         str(fixture / f"postgres-{port}.log"), "-o", f"-h '' -k {fixture} -p {port}", "start"]
                subprocess.run(start, check=True, cwd=repo)
                pid = (cluster / "postmaster.pid").read_text().splitlines()[0]
                print(f"Owned PostgreSQL pid={pid} cwd={repo} socket={fixture}/.s.PGSQL.{port} cmd={start}", flush=True)
            redis_command = ["redis-server", "--port", "0", "--unixsocket", str(redis_socket),
                             "--unixsocketperm", "700", "--save", "", "--appendonly", "no"]
            with (fixture / "redis.log").open("w") as log:
                redis_process = subprocess.Popen(redis_command, cwd=fixture, stdout=log, stderr=log)
            print(f"Owned Redis pid={redis_process.pid} cwd={fixture} socket={redis_socket} cmd={redis_command}", flush=True)
            for _ in range(50):
                if redis_socket.exists() or redis_process.poll() is not None:
                    break
                time.sleep(0.1)
            if not redis_socket.exists():
                raise RuntimeError("owned Redis did not bind its Unix socket")
            def dsn(port):
                return f"host={fixture} port={port} user=storage_control_test dbname=postgres sslmode=disable"

            env = dict(os.environ, POWERX_STORAGE_DATASET_TEST_REDIS_SOCKET=str(redis_socket),
                       POWERX_STORAGE_CONTROL_TEST_DSN=dsn(clusters[0][1]),
                       POWERX_STORAGE_DATASET_TEST_EXTERNAL_DSN=dsn(clusters[1][1]))
            pattern = ("^Test(StorageDatasetsNativeSTSAndHTTP|"
                       "DatasetContractRejectsInvalidBoundsCountsAndLease|"
                       "DatasetCatalogSyntheticPluginNativePostgres|"
                       "SystemTableRetentionRuntimeNativePostgres|ManagedDatasetBinding|DatasetArchiveVersionedReadbackAndReplay)$")
            with minio_fixture(fixture) if options.with_minio else nullcontext("") as archive_config:
                env["POWERX_STORAGE_DATASET_TEST_S3_CONFIG"] = archive_config
                subprocess.run(
                    ["go", "test", "-race", "-p", "2", "-count=1", "-v", "./internal/bootstrap",
                     "./internal/service/storage_governance", "./internal/infra/plugin/manager", "./internal/service/archive_ops", "-run", pattern],
                    cwd=repo / "backend", env=env, timeout=210, check=True,
                )
        finally:
            stop_owned(redis_process)
            for cluster, port in reversed(clusters):
                if not (cluster / "PG_VERSION").exists():
                    continue
                if (cluster / "postmaster.pid").exists():
                    subprocess.run([pg("pg_ctl"), "-D", str(cluster), "-m", "fast", "-w", "stop"], check=True)
                status = subprocess.run([pg("pg_ctl"), "-D", str(cluster), "status"], capture_output=True)
                if (fixture / f".s.PGSQL.{port}").exists() or status.returncode != 3:
                    raise RuntimeError("owned PostgreSQL did not stop")
            if redis_socket.exists():
                raise RuntimeError("owned Redis socket remained")
            print("Owned PostgreSQL and Redis stopped and sockets released.", flush=True)


if __name__ == "__main__":
    main()
