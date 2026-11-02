#!/usr/bin/env python3
"""Temporary host log retention. Never changes database rows or business files."""
import argparse
import datetime as dt
import fcntl
import glob
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import time
import custody_policy

CONTROL_PROTOCOL = 2

GIB = 1024 ** 3
# Audit sources have a separate custody lifecycle. Ordinary retention must never
# delete them, even under capacity pressure or after the local retention window.
AUDIT_SOURCES = frozenset({"postgres", "postgres-native", "prod-audit", "dev-audit"})
RULES = [
    ("postgres", "/var/log/postgresql", r"postgresql-16-main\.log\.\d+\.gz", 2 * GIB),
    ("postgres-native", "/var/log/postgresql/powerx-audit", r"postgresql-audit-\d{4}-\d{2}-\d{2}_\d{6}\.log", 2 * GIB),
    ("syslog", "/var/log", r"syslog\.\d+\.gz", 512 * 1024 ** 2),
    ("nginx", "/var/log/nginx", r"(?:access|error)\.log\.\d+\.gz", 256 * 1024 ** 2),
    ("prod-audit", "/opt/powerx/backend/logs/audit", r"audit_event-.*\.log(?:\.gz)?", 512 * 1024 ** 2),
    ("dev-audit", "/opt/powerx-dev/backend/logs/audit", r"audit_event-.*\.log(?:\.gz)?", 512 * 1024 ** 2),
]


def filesystem(path="/"):
    s = os.statvfs(path)
    total = s.f_blocks * s.f_frsize
    used = (s.f_blocks - s.f_bfree) * s.f_frsize
    return {"total_bytes": total, "used_bytes": used,
            "available_bytes": s.f_bavail * s.f_frsize,
            "reserved_bytes": (s.f_bfree - s.f_bavail) * s.f_frsize,
            "used_ratio": used / total}


def command(args, timeout=120):
    p = subprocess.run(args, text=True, capture_output=True, timeout=timeout)
    if p.returncode:
        # Never include command output: SQL errors and logs may contain business data.
        raise RuntimeError(f"{Path(args[0]).name} exited {p.returncode}")
    return p.stdout


def open_inodes():
    result = set()
    for fd in glob.glob("/proc/[0-9]*/fd/*"):
        try:
            s = os.stat(fd)
            result.add((s.st_dev, s.st_ino))
        except FileNotFoundError:
            pass
        except PermissionError as e:
            raise RuntimeError("cannot inspect process descriptors; refusing retention") from e
        except OSError:
            # Descriptors can disappear while processes run.
            continue
    return result


def candidates(directory, pattern, opened):
    """Resolve a deployed release symlink once; never follow candidate symlinks."""
    base = Path(directory).resolve(strict=True)
    result = []
    for p in base.iterdir():
        s = p.lstat()
        if (re.fullmatch(pattern, p.name) and stat.S_ISREG(s.st_mode)
                and s.st_nlink == 1 and (s.st_dev, s.st_ino) not in opened):
            result.append((s.st_mtime, p, s))
    return sorted(result, key=lambda x: (x[0], str(x[1])))


def select_expired(items, now, retention_days, budget):
    """Age limit first, then size limit; size pressure always keeps latest 24h."""
    remaining = sum(s.st_blocks * 512 for _, _, s in items)
    chosen = []
    for mtime, p, s in items:
        reason = None
        if mtime < now - retention_days * 86400:
            reason = "retention_expired"
        elif remaining > budget and mtime < now - 86400:
            reason = "closed_log_budget"
        if reason:
            chosen.append((p, s, reason))
            remaining -= s.st_blocks * 512
    return chosen


def safe_unlink(p, original):
    """Revalidate identity/content metadata and open handles immediately before unlink."""
    current = p.lstat()
    keys = ("st_dev", "st_ino", "st_size", "st_mtime_ns", "st_ctime_ns", "st_nlink")
    if not stat.S_ISREG(current.st_mode) or any(
            getattr(current, k) != getattr(original, k) for k in keys):
        raise RuntimeError("candidate changed; refusing unlink")
    if (current.st_dev, current.st_ino) in open_inodes():
        raise RuntimeError("candidate became open; refusing unlink")
    p.unlink()
    return current.st_blocks * 512


def atomic_json(path, value):
    tmp = path.with_suffix(".tmp")
    with tmp.open("w") as f:
        json.dump(value, f, ensure_ascii=False, indent=2)
        f.write("\n")
        f.flush()
        os.fsync(f.fileno())
    os.chmod(tmp, 0o600)
    tmp.replace(path)


def retain_logs(apply, days, report):
    opened = open_inodes()
    for label, directory, pattern, budget in RULES:
        try:
            items = candidates(directory, pattern, opened)
        except FileNotFoundError:
            continue
        selected = select_expired(items, time.time(), days, budget)
        controlled = custody_policy.state_for(directory)
        if controlled['managed']:
            report.setdefault('audit_archive', {})[label] = {
                'state': controlled['reason'], 'closed_files': len(items),
                'allocated_bytes': sum(s.st_blocks * 512 for _, _, s in items),
                'budget_bytes': budget, 'over_budget': sum(s.st_blocks * 512 for _, _, s in items) > budget,
                'blocked_deletions': 0, 'owner': 'powerx'}
            continue
        if label in AUDIT_SOURCES:
            quarantine = Path(directory).resolve() / '.powerx-audit-custody'
            if quarantine.exists() and (quarantine.is_symlink() or any(quarantine.iterdir())):
                raise RuntimeError('audit_quarantine_requires_reconciliation')
            backlog = report.setdefault("audit_archive", {})
            backlog[label] = {"state": "archive_required",
                              "closed_files": len(items),
                              "allocated_bytes": sum(s.st_blocks * 512 for _, _, s in items),
                              "budget_bytes": budget,
                              "over_budget": sum(s.st_blocks * 512 for _, _, s in items) > budget,
                              "blocked_deletions": len(selected)}
        for path, s, reason in selected:
            entry = {"source": label, "path": str(path), "reason": reason,
                     "allocated_bytes": s.st_blocks * 512, "deleted": False}
            report["files"].append(entry)
            if label in AUDIT_SOURCES:
                entry["blocked_reason"] = "verified_offhost_archive_required"
                continue
            if apply:
                safe_unlink(path, s)
                entry["deleted"] = True


def compress_closed_audit_logs(apply, report):
    for label, directory, _, _ in RULES:
        if label not in ("prod-audit", "dev-audit"):
            continue
        if custody_policy.state_for(directory)['managed']:
            continue
        try:
            items = candidates(directory, r"audit_event-\d{4}-\d{2}-\d{2}.*\.log", open_inodes())
        except FileNotFoundError:
            continue
        for mtime, p, original in items[:8]:
            if mtime >= time.time() - 86400 or p.with_name(p.name + ".gz").exists():
                continue
            entry = {"source": label, "path": str(p), "compressed": False}
            report["compression"].append(entry)
            if apply:
                current = p.lstat()
                if (current.st_ino, current.st_size, current.st_mtime_ns) != (
                        original.st_ino, original.st_size, original.st_mtime_ns):
                    raise RuntimeError("audit candidate changed; refusing compression")
                if (current.st_dev, current.st_ino) in open_inodes():
                    raise RuntimeError("audit candidate became open; refusing compression")
                command(["gzip", "-1", "--", str(p)], 60)
                entry["compressed"] = True


def database_sizes():
    raw = command(["runuser", "-u", "postgres", "--", "psql", "-X", "-A", "-t",
                   "-v", "ON_ERROR_STOP=1", "-d", "postgres", "-c",
                   "SET statement_timeout='10s'; SELECT json_object_agg(datname, "
                   "pg_database_size(oid)) FROM pg_database WHERE NOT datistemplate;"], 20)
    return json.loads(next(line for line in raw.splitlines() if line.startswith("{")))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="default is read-only preview")
    parser.add_argument("--state-dir", default="/var/lib/powerx-space-guard")
    parser.add_argument("--retention-days", type=int, default=7)
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error("run as root so open-file checks are complete")
    if not 7 <= args.retention_days <= 90:
        parser.error("retention-days must be between 7 and 90")
    os.umask(0o077)
    state = Path(args.state_dir)
    if args.apply:
        state.mkdir(parents=True, exist_ok=True, mode=0o700)
        lock = (state / "guard.lock").open("a")
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print(json.dumps({"status": "skipped", "reason": "already_running"}))
            return 0
    previous = {}
    if (state / "latest.json").exists():
        previous = json.loads((state / "latest.json").read_text())
    now = time.time()
    report = {"started_at": dt.datetime.now(dt.timezone.utc).isoformat(), "epoch": now,
              "apply": args.apply, "policy": {"retention_days": args.retention_days,
              "minimum_size_retention_hours": 24, "journal_budget_bytes": GIB,
              "warn_available_bytes": 12 * GIB, "critical_available_bytes": 8 * GIB,
              "critical_used_ratio": 0.88}, "before": filesystem(), "files": [],
              "compression": [], "errors": []}

    def phase(name, func):
        try:
            return func()
        except Exception as e:
            report["errors"].append({"phase": name, "error": type(e).__name__,
                                     "reason": str(e) if isinstance(e, RuntimeError) else "operation_failed"})
            return None

    if args.apply:
        # Shares the normal system logrotate state/lock; no competing rotation state.
        phase("postgres_rotation", lambda: command([
            "logrotate", "/etc/logrotate.d/postgresql-common",
            "/etc/logrotate.d/powerx-syslog-temporary"], 180))
        journal_rotated = previous.get("journal_rotated_epoch", 0)
        if now - journal_rotated >= 3600:
            if phase("journal_rotation", lambda: command(["journalctl", "--rotate"], 30)) is not None:
                journal_rotated = now
        report["journal_rotated_epoch"] = journal_rotated
        if custody_policy.managed_tree('/var/log/journal'):
            report['journal_owner'] = 'powerx'
        else:
            phase("journal_retention", lambda: command([
                "journalctl", "--vacuum-size=1G", f"--vacuum-time={args.retention_days}days"], 120))
    phase("closed_audit_compression", lambda: compress_closed_audit_logs(args.apply, report))
    phase("closed_file_retention", lambda: retain_logs(args.apply, args.retention_days, report))
    last_db = previous.get("database_checked_epoch", 0)
    if now - last_db >= 3600:
        sizes = phase("database_read_only", database_sizes)
        if sizes is not None:
            report["database_sizes_bytes"] = sizes
            report["database_checked_epoch"] = now
    else:
        report["database_sizes_bytes"] = previous.get("database_sizes_bytes", {})
        report["database_checked_epoch"] = last_db
    report["after"] = filesystem()
    after = report["after"]
    if report["errors"]:
        report["status"] = "failed"
    elif after["available_bytes"] < 8 * GIB or after["used_ratio"] >= 0.88:
        report["status"] = "critical"
    elif after["available_bytes"] < 12 * GIB:
        report["status"] = "warning"
    elif any(v["blocked_deletions"] or v["over_budget"]
             for v in report.get("audit_archive", {}).values()):
        report["status"] = "warning"
    else:
        report["status"] = "ok"
    report["observed_available_change_bytes"] = after["available_bytes"] - report["before"]["available_bytes"]
    report["database_rows_changed"] = 0
    report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
    if args.apply:
        runs = state / "runs"
        runs.mkdir(mode=0o700, exist_ok=True)
        atomic_json(runs / f"{time.time_ns()}.json", report)
        atomic_json(state / "latest.json", report)
        # The guard's own execution records are bounded as well.
        for path in sorted(runs.glob("*.json"))[:-2016]:
            path.unlink()
    print(json.dumps(report, ensure_ascii=False))
    return 2 if report["status"] in ("critical", "failed") else 0


if __name__ == "__main__":
    sys.exit(main())
