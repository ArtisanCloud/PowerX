"""API/归属记录/回执的隔离回归，不触发实际 Actions 或 Registry。"""
import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

import build_contract as c
from test_build_contract import environment, request, scenario


def decision(ctx, owner=None):
    return {"schema_version": "powerx.build-decision/v1", "request_id": ctx["request_id"],
            "input_sha256": ctx["input_sha256"], "run": ctx["run"],
            "decision": "duplicate" if owner else "owner",
            "owner": owner or {"run_id": ctx["run"]["run_id"], "attempt": ctx["run"]["attempt"],
                              "input_sha256": ctx["input_sha256"]}}


class ClaimAPI:
    def __init__(self):
        self.runs = {}
        self.attempts = {}
        self.artifacts = {}
        self.archives = {}
        self.jobs = {}
        self.calls = []

    def add_artifact(self, run_id, name, files):
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            for filename, raw in files.items():
                archive.writestr(filename, raw)
        artifact_id = len(self.archives) + 1
        raw = data.getvalue()
        self.archives[artifact_id] = raw
        self.artifacts.setdefault(run_id, []).append(
            {"id": artifact_id, "name": name, "expired": False, "digest": "sha256:" + c.digest(raw)})

    def add(self, run_id, attempt=1, *, owner=None, state="completed", status="succeeded", mode="verify"):
        ctx = request(mode, environment(run=run_id, attempt=attempt))
        meta = {"id": run_id, "run_attempt": attempt, "status": state,
                "conclusion": ("success" if status in {"succeeded", "duplicate"} else "failure") if state == "completed" else None,
                "event": "workflow_dispatch", "path": c.WORKFLOW,
                "repository": {"full_name": c.REPOSITORY}, "head_sha": ctx["actual_source_sha"],
                "head_branch": "develop", "display_title": "PowerX request=" + ctx["request_id"] + " mode=" + mode}
        self.runs[run_id] = meta
        self.attempts[(run_id, attempt)] = copy.deepcopy(meta)
        self.jobs[(run_id, attempt)] = [] if state != "completed" else [{"name": "preflight", "status": "completed", "started_at": "2026-11-12T00:00:00Z"}]
        if state != "completed":
            return ctx
        suffix = "-" + str(run_id) + "-" + str(attempt)
        raw_context = json.dumps(ctx, indent=2, sort_keys=True).encode() + b"\n"
        claim = decision(ctx, owner)
        raw_claim = json.dumps(claim, indent=2, sort_keys=True).encode() + b"\n"
        self.add_artifact(run_id, "powerx-build-request" + suffix, {"request-context.json": raw_context})
        self.add_artifact(run_id, "powerx-request-decision" + suffix, {"request-decision.json": raw_claim})
        with tempfile.TemporaryDirectory() as temp:
            env, jobs = scenario(Path(temp), "verify")
            receipt = c.make_receipt(temp, env, jobs)
        receipt.update(run=ctx["run"], request=ctx, status=status, deployable=False, duplicate_of=owner,
                       definition_sha256=ctx["definition_sha256"], phase_evidence={})
        receipt["artifacts"] = [
            {"artifact": "powerx-build-request" + suffix, "path": "request-context.json",
             "sha256": c.digest(raw_context), "size": len(raw_context)},
            {"artifact": "powerx-request-decision" + suffix, "path": "request-decision.json",
             "sha256": c.digest(raw_claim), "size": len(raw_claim)}]
        raw_receipt = json.dumps(receipt).encode()
        self.add_artifact(run_id, "powerx-build-receipt" + suffix,
                          {"powerx-build-receipt.json": raw_receipt,
                           "powerx-build-receipt.json.sha256":
                           (c.digest(raw_receipt) + "  powerx-build-receipt.json\n").encode()})
        return ctx

    def pages(self, path, field):
        self.calls.append(("pages", path))
        if field == "workflow_runs":
            return list(copy.deepcopy(self.runs).values())
        run_id = int(path.split("/runs/")[1].split("/")[0])
        if field == "artifacts":
            return copy.deepcopy(self.artifacts.get(run_id, []))
        attempt = int(path.split("/attempts/")[1].split("/")[0])
        return copy.deepcopy(self.jobs[(run_id, attempt)])

    def request(self, path, **kwargs):
        self.calls.append(("request", path))
        if "/artifacts/" in path:
            return self.archives[int(path.split("/artifacts/")[1].split("/")[0])]
        run_id = int(path.split("/runs/")[1].split("/")[0])
        if "/attempts/" in path:
            return copy.deepcopy(self.attempts[(run_id, int(path.rsplit("/", 1)[1]))])
        return copy.deepcopy(self.runs[run_id])


class RequestClaimTests(unittest.TestCase):
    def test_higher_run_processed_first_is_reused(self):
        api = ClaimAPI()
        api.add(101)
        found = c.deduplicate(request("verify"), api)
        self.assertEqual(found, {"run_id": 101, "attempt": 1, "input_sha256": request("verify")["input_sha256"]})

    def test_queued_lower_run_cannot_claim_ownership(self):
        for state in ("queued", "in_progress", "waiting"):
            with self.subTest(state=state):
                api = ClaimAPI()
                api.add(99, state=state)
                self.assertIsNone(c.deduplicate(request("verify"), api))

    def test_processed_same_id_changed_input_is_rejected(self):
        api = ClaimAPI()
        api.add(101)
        for mode in ("build", "release"):
            with self.assertRaisesRegex(ValueError, "BUILD_REQUEST_ID_CONFLICT"):
                c.deduplicate(request(mode), api)

    def test_missing_corrupt_or_unknown_result_blocks_replay(self):
        for case in ("missing-receipt", "missing-decision", "missing-context", "corrupt", "unknown",
                     "unknown-conclusion", "preflight-started"):
            with self.subTest(case=case):
                api = ClaimAPI()
                api.add(101)
                if case.startswith("missing-"):
                    word = {"missing-receipt": "receipt", "missing-decision": "decision",
                            "missing-context": "build-request"}[case]
                    api.artifacts[101] = [item for item in api.artifacts[101] if word not in item["name"]]
                elif case == "corrupt":
                    api.archives[3] = b"broken"
                elif case == "unknown":
                    api.runs[101]["status"] = "in_progress"
                    api.runs[101]["conclusion"] = None
                elif case == "unknown-conclusion":
                    api.runs[101]["conclusion"] = "neutral"
                else:
                    api = ClaimAPI()
                    api.add(101, state="in_progress")
                    api.jobs[(101, 1)] = [{"name": "preflight", "status": "in_progress", "started_at": "2026-11-12T00:00:00Z"}]
                with self.assertRaises(ValueError):
                    c.deduplicate(request("verify"), api)

    def test_duplicate_points_directly_to_confirmed_owner(self):
        api = ClaimAPI()
        owner = api.add(101)
        pointer = decision(owner)["owner"]
        api.add(99, owner=pointer, status="duplicate")
        self.assertEqual(c.deduplicate(request("verify"), api), pointer)

    def test_multiple_owners_or_duplicate_chains_are_rejected(self):
        api = ClaimAPI()
        api.add(99)
        api.add(101)
        with self.assertRaisesRegex(ValueError, "BUILD_REQUEST_OWNERSHIP_CONFLICT"):
            c.deduplicate(request("verify"), api)
        api = ClaimAPI()
        owner = api.add(101)
        api.add(99, owner=decision(owner)["owner"], status="duplicate")
        api.add(102, owner={"run_id": 99, "attempt": 1, "input_sha256": owner["input_sha256"]}, status="duplicate")
        with self.assertRaisesRegex(ValueError, "BUILD_DUPLICATE_CHAIN_INVALID"):
            c.deduplicate(request("verify"), api)

    def test_owner_rerun_does_not_reuse_its_own_older_attempt(self):
        api = ClaimAPI()
        original = api.add(100)
        api.add(101, owner=decision(original)["owner"], status="duplicate")
        self.assertIsNone(c.deduplicate(request("verify", environment(attempt=2)), api))

    def test_duplicate_cycle_is_rejected(self):
        api = ClaimAPI()
        fingerprint = request("verify")["input_sha256"]
        api.add(99, owner={"run_id": 101, "attempt": 1, "input_sha256": fingerprint}, status="duplicate")
        api.add(101, owner={"run_id": 99, "attempt": 1, "input_sha256": fingerprint}, status="duplicate")
        with self.assertRaisesRegex(ValueError, "BUILD_DUPLICATE_CHAIN_INVALID"):
            c.deduplicate(request("verify"), api)

    def test_manual_push_like_id_does_not_bypass_history(self):
        api = ClaimAPI()
        c.deduplicate(request("verify", request_id="push-100"), api)
        self.assertTrue(any("workflows/publish-container.yml/runs" in path for _, path in api.calls))

    def test_receipt_inner_checksum_is_verified(self):
        api = ClaimAPI()
        api.add(101)
        artifact = next(item for item in api.artifacts[101] if "build-receipt" in item["name"])
        data = io.BytesIO()
        with zipfile.ZipFile(io.BytesIO(api.archives[artifact["id"]])) as original, zipfile.ZipFile(data, "w") as changed:
            for filename in original.namelist():
                raw = original.read(filename)
                changed.writestr(filename, raw + b" " if filename.endswith(".json") else raw)
        api.archives[artifact["id"]] = data.getvalue()
        artifact["digest"] = "sha256:" + c.digest(data.getvalue())
        with self.assertRaisesRegex(ValueError, "BUILD_PREVIOUS_RECEIPT_CHECKSUM_MISMATCH"):
            c.deduplicate(request("verify"), api)

    def test_new_request_uses_complete_latest_owner_attempt(self):
        api = ClaimAPI()
        first = api.add(101)
        api.add(99, owner=decision(first)["owner"], status="duplicate")
        api.add(101, attempt=2)
        found = c.deduplicate(request("verify"), api)
        self.assertEqual((found["run_id"], found["attempt"]), (101, 2))
        api.runs[101]["status"] = "queued"
        api.runs[101]["conclusion"] = None
        with self.assertRaises(ValueError):
            c.deduplicate(request("verify"), api)


if __name__ == "__main__":
    unittest.main()
