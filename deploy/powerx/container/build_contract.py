"""Core 构建请求、阶段证据和独立回执；不执行线上部署。"""
import argparse
from datetime import datetime, timezone
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
import zipfile

import upgrade_release

ROOT = Path(__file__).resolve().parents[3]
REPOSITORY = "ArtisanCloud/PowerX"
WORKFLOW = ".github/workflows/publish-container.yml"
CHANNELS = {"refs/heads/develop": "development", "refs/heads/main": "stable", "refs/heads/master": "stable"}
MODES = {"verify", "build", "release"}
INPUTS = {"request_id", "expected_source_sha", "build_mode"}
PHASES = {"preflight", "checks", "images", "upgrade-amd64", "upgrade-arm64", "metadata", "release"}
RESULTS = {"success", "failure", "cancelled", "skipped"}
REQUEST_SCHEMA = "powerx.build-request/v1"
RECEIPT_SCHEMA = "powerx.build-receipt/v1"


def require(condition, code):
    if not condition:
        raise ValueError(code)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "BUILD_DUPLICATE_JSON_KEY")
        result[key] = value
    return result


def loads(raw):
    return json.loads(raw, object_pairs_hook=unique_object)


def read(path):
    return loads(Path(path).read_bytes())


def write(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def identity(env):
    require(env.get("GITHUB_REPOSITORY") == REPOSITORY, "BUILD_REPOSITORY_UNSUPPORTED")
    require(re.fullmatch(r"[1-9][0-9]{0,19}", env.get("GITHUB_RUN_ID", "")), "BUILD_RUN_INVALID")
    require(re.fullmatch(r"[1-9][0-9]{0,8}", env.get("GITHUB_RUN_ATTEMPT", "")), "BUILD_ATTEMPT_INVALID")
    sha = env.get("GITHUB_SHA", "")
    require(upgrade_release.SHA.fullmatch(sha), "BUILD_SOURCE_SHA_INVALID")
    ref = env.get("GITHUB_REF", "")
    require(ref in CHANNELS, "BUILD_REF_UNSUPPORTED")
    require(env.get("GITHUB_WORKFLOW_REF") == REPOSITORY + "/" + WORKFLOW + "@" + ref,
            "BUILD_WORKFLOW_IDENTITY_INVALID")
    require(env.get("GITHUB_WORKFLOW_SHA") == sha, "BUILD_WORKFLOW_SHA_MISMATCH")
    return {"repository": REPOSITORY, "workflow_path": WORKFLOW, "workflow_ref": env["GITHUB_WORKFLOW_REF"],
            "workflow_sha": sha, "run_id": int(env["GITHUB_RUN_ID"]), "attempt": int(env["GITHUB_RUN_ATTEMPT"]),
            "head_sha": sha, "ref": ref}


def prepare(event, env, actual_sha):
    run = identity(env)
    event_name = env.get("GITHUB_EVENT_NAME")
    require(event_name in {"push", "workflow_dispatch"}, "BUILD_EVENT_UNSUPPORTED")
    if event_name == "workflow_dispatch":
        inputs = event.get("inputs")
        require(isinstance(inputs, dict) and set(inputs) == INPUTS, "BUILD_INPUT_KEYS_INVALID")
        request_id, expected, mode = (inputs.get(key) for key in ("request_id", "expected_source_sha", "build_mode"))
    else:
        require(event.get("ref") == run["ref"] and event.get("after") == run["head_sha"],
                "BUILD_PUSH_IDENTITY_INVALID")
        request_id, expected, mode = "push-" + str(run["run_id"]), run["head_sha"], "verify"
    require(isinstance(request_id, str) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{7,127}", request_id),
            "BUILD_REQUEST_ID_INVALID")
    require(isinstance(expected, str) and upgrade_release.SHA.fullmatch(expected), "BUILD_EXPECTED_SHA_INVALID")
    require(isinstance(mode, str) and mode in MODES, "BUILD_MODE_UNSUPPORTED")
    require(expected == actual_sha == run["head_sha"], "BUILD_SOURCE_SHA_MISMATCH")
    inputs = {"request_id": request_id, "expected_source_sha": expected, "build_mode": mode, "ref": run["ref"]}
    return {"schema_version": REQUEST_SCHEMA, **inputs, "input_sha256": digest(canonical(inputs)),
            "actual_source_sha": actual_sha, "channel": CHANNELS[run["ref"]], "run": run,
            "definition_sha256": digest((ROOT / WORKFLOW).read_bytes()),
            "build_time": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}


def validate_request(request):
    require(isinstance(request, dict) and request.get("schema_version") == REQUEST_SCHEMA, "BUILD_REQUEST_INVALID")
    run = request.get("run", {})
    env = {"GITHUB_REPOSITORY": run.get("repository"), "GITHUB_SHA": run.get("head_sha"),
           "GITHUB_REF": run.get("ref"), "GITHUB_WORKFLOW_REF": run.get("workflow_ref"),
           "GITHUB_WORKFLOW_SHA": run.get("workflow_sha"), "GITHUB_RUN_ID": str(run.get("run_id")),
           "GITHUB_RUN_ATTEMPT": str(run.get("attempt")), "GITHUB_EVENT_NAME": "workflow_dispatch"}
    inputs = {key: request.get(key) for key in INPUTS}
    rebuilt = prepare({"inputs": inputs}, env, request.get("actual_source_sha"))
    require(set(request) == set(rebuilt) and run == rebuilt["run"], "BUILD_REQUEST_FIELDS_INVALID")
    require(request.get("input_sha256") == rebuilt["input_sha256"] and
            request.get("channel") == rebuilt["channel"] and request.get("ref") == run.get("ref"),
            "BUILD_REQUEST_DIGEST_MISMATCH")
    require(re.fullmatch(r"[a-f0-9]{64}", request.get("definition_sha256", "")), "BUILD_DEFINITION_INVALID")
    require(isinstance(request.get("build_time"), str) and
            re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", request["build_time"]), "BUILD_TIME_INVALID")
    return request


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        require(urllib.parse.urlparse(newurl).scheme == "https", "BUILD_REDIRECT_INVALID")
        result = super().redirect_request(req, fp, code, msg, headers, newurl)
        # GitHub 的制品下载可能跳到对象存储，不能将仓库凭据转发过去。
        if result is not None:
            result.remove_header("Authorization")
        return result


class GitHub:
    def __init__(self, token):
        require(bool(token), "BUILD_GITHUB_TOKEN_MISSING")
        self.token = token
        self.opener = urllib.request.build_opener(SafeRedirect())

    def request(self, path, method="GET", data=None, accept="application/vnd.github+json", limit=2**20):
        require(path.startswith("/") and not path.startswith("//"), "BUILD_GITHUB_PATH_INVALID")
        request = urllib.request.Request("https://api.github.com" + path, method=method,
            data=None if data is None else canonical(data),
            headers={"Authorization": "Bearer " + self.token, "Accept": accept,
                     "X-GitHub-Api-Version": "2022-11-28", "Content-Type": "application/json"})
        try:
            with self.opener.open(request, timeout=60) as response:
                raw = response.read(limit + 1)
            require(len(raw) <= limit, "BUILD_GITHUB_RESPONSE_TOO_LARGE")
            return raw if accept == "application/octet-stream" else loads(raw)
        except urllib.error.HTTPError as error:
            if error.code == 404:
                return None
            raise ValueError("BUILD_GITHUB_HTTP_" + str(error.code)) from None

    def pages(self, path, field):
        result = []
        for page in range(1, 11):
            separator = "&" if "?" in path else "?"
            data = self.request(path + separator + "per_page=100&page=" + str(page))
            items = data if isinstance(data, list) and field in {"releases", "assets"} else (
                data.get(field) if isinstance(data, dict) else None)
            require(isinstance(items, list), "BUILD_GITHUB_LIST_INVALID")
            result.extend(items)
            if len(items) < 100:
                require(not isinstance(data, dict) or data.get("total_count", len(result)) == len(result),
                        "BUILD_GITHUB_LIST_INCOMPLETE")
                return result
        raise ValueError("BUILD_GITHUB_LIST_LIMIT")

    def upload(self, release_id, name, raw):
        require(isinstance(release_id, int) and release_id > 0 and
                re.fullmatch(r"[a-z0-9.-]+", name), "BUILD_RELEASE_ASSET_INVALID")
        request = urllib.request.Request("https://uploads.github.com/repos/" + REPOSITORY +
            "/releases/" + str(release_id) + "/assets?name=" + name, method="POST", data=raw,
            headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/octet-stream",
                     "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"})
        with self.opener.open(request, timeout=120) as response:
            return loads(response.read(2**20))


def validate_schema(value, filename, error_code):
    import jsonschema
    try:
        jsonschema.validate(value, read(ROOT / "docs/contracts" / filename))
    except (jsonschema.ValidationError, jsonschema.SchemaError):
        raise ValueError(error_code) from None


def decision_record(request, duplicate=None):
    validate_request(request)
    return {"schema_version": "powerx.build-decision/v1", "request_id": request["request_id"],
            "input_sha256": request["input_sha256"], "run": request["run"],
            "decision": "duplicate" if duplicate else "owner",
            "owner": duplicate or {"run_id": request["run"]["run_id"], "attempt": request["run"]["attempt"],
                                   "input_sha256": request["input_sha256"]}}


def validate_decision(value, request):
    validate_schema(value, "powerx-build-decision.schema.json", "BUILD_DECISION_INVALID")
    require(value["run"] == request["run"] and value["request_id"] == request["request_id"] and
            value["input_sha256"] == request["input_sha256"] == value["owner"]["input_sha256"],
            "BUILD_DECISION_REQUEST_MISMATCH")
    if value["decision"] == "owner":
        require(value == decision_record(request), "BUILD_DECISION_OWNER_INVALID")
    else:
        require(value["owner"]["run_id"] != request["run"]["run_id"], "BUILD_DUPLICATE_CHAIN_INVALID")
    return value


class RequestEvidence:
    """只读核对处理记录；run ID 的大小和列表顺序都不代表实际归属。"""
    def __init__(self, request, github):
        self.current, self.github = request, github
        self.latest_cache, self.attempt_cache, self.artifact_cache, self.processed_cache = {}, {}, {}, {}

    def metadata(self, run_id, attempt=None):
        cache = self.latest_cache if attempt is None else self.attempt_cache
        key = run_id if attempt is None else (run_id, attempt)
        if key not in cache:
            path = "/repos/" + REPOSITORY + "/actions/runs/" + str(run_id)
            if attempt is not None:
                path += "/attempts/" + str(attempt)
            value = self.github.request(path)
            require(isinstance(value, dict) and value.get("id") == run_id and
                    type(value.get("run_attempt")) is int and value["run_attempt"] > 0 and
                    (attempt is None or value["run_attempt"] == attempt) and
                    value.get("event") == "workflow_dispatch" and
                    value.get("path", "").split("@")[0] == WORKFLOW and
                    value.get("repository", {}).get("full_name") == REPOSITORY,
                    "BUILD_PREVIOUS_RUN_INVALID")
            cache[key] = value
        return cache[key]

    def artifact(self, meta, prefix, filename, optional=False, checksum=False):
        run_id, attempt = meta["id"], meta["run_attempt"]
        if run_id not in self.artifact_cache:
            self.artifact_cache[run_id] = self.github.pages(
                "/repos/" + REPOSITORY + "/actions/runs/" + str(run_id) + "/artifacts", "artifacts")
        name = prefix + "-" + str(run_id) + "-" + str(attempt)
        matches = [item for item in self.artifact_cache[run_id] if item.get("name") == name]
        if not matches and optional:
            return None
        require(len(matches) == 1 and not matches[0].get("expired"), "BUILD_PREVIOUS_EVIDENCE_UNCONFIRMED")
        item = matches[0]
        require(type(item.get("id")) is int and item["id"] > 0, "BUILD_PREVIOUS_ARTIFACT_INVALID")
        raw = self.github.request("/repos/" + REPOSITORY + "/actions/artifacts/" + str(item["id"]) + "/zip",
                                  accept="application/octet-stream", limit=4 * 2**20)
        require(isinstance(raw, bytes) and len(raw) <= 4 * 2**20, "BUILD_PREVIOUS_ARTIFACT_INVALID")
        if item.get("digest"):
            require(item["digest"] == "sha256:" + digest(raw), "BUILD_PREVIOUS_ARTIFACT_DIGEST_MISMATCH")
        expected = {filename, filename + ".sha256"} if checksum else {filename}
        try:
            with zipfile.ZipFile(io.BytesIO(raw)) as archive:
                require(len(archive.namelist()) == len(expected) and set(archive.namelist()) == expected and
                        sum(info.file_size for info in archive.infolist()) <= 4 * 2**20,
                        "BUILD_PREVIOUS_ARTIFACT_INVALID")
                content = archive.read(filename)
                if checksum:
                    require(archive.read(filename + ".sha256").decode().strip() == digest(content) + "  " + filename,
                            "BUILD_PREVIOUS_RECEIPT_CHECKSUM_MISMATCH")
        except (zipfile.BadZipFile, UnicodeDecodeError):
            raise ValueError("BUILD_PREVIOUS_ARTIFACT_INVALID") from None
        return {"value": loads(content), "artifact": name, "path": filename,
                "sha256": digest(content), "size": len(content)}

    def unstarted(self, meta, record):
        if record is not None or meta["run_attempt"] != 1 or meta.get("status") not in {
                "queued", "requested", "waiting", "pending", "in_progress"}:
            return False
        jobs = self.github.pages("/repos/" + REPOSITORY + "/actions/runs/" + str(meta["id"]) +
            "/attempts/1/jobs", "jobs")
        return all(job.get("status") in {"queued", "requested", "waiting", "pending"} and
                   not job.get("started_at") and not job.get("conclusion") for job in jobs)

    def processed(self, meta):
        key = (meta["id"], meta["run_attempt"])
        if key in self.processed_cache:
            return self.processed_cache[key]
        record = self.artifact(meta, "powerx-request-decision", "request-decision.json", optional=True)
        if self.unstarted(meta, record):
            return None
        require(meta.get("status") == "completed" and meta.get("conclusion") is not None and record is not None,
                "BUILD_PREVIOUS_RESULT_UNCONFIRMED")
        context = self.artifact(meta, "powerx-build-request", "request-context.json")
        previous = validate_request(context["value"])
        require(previous["run"]["run_id"] == meta["id"] and previous["run"]["attempt"] == meta["run_attempt"] and
                previous["actual_source_sha"] == meta.get("head_sha") and
                previous["ref"] == "refs/heads/" + str(meta.get("head_branch")) and
                previous["request_id"] == self.current["request_id"], "BUILD_PREVIOUS_RUN_INVALID")
        value = validate_decision(record["value"], previous)
        require(previous["input_sha256"] == self.current["input_sha256"], "BUILD_REQUEST_ID_CONFLICT")
        require(previous["definition_sha256"] == self.current["definition_sha256"],
                "BUILD_PREVIOUS_DEFINITION_MISMATCH")
        receipt = self.artifact(meta, "powerx-build-receipt", "powerx-build-receipt.json", checksum=True)["value"]
        validate_schema(receipt, "powerx-build-receipt.schema.json", "BUILD_PREVIOUS_RECEIPT_INVALID")
        require(receipt["request"] == previous and receipt["run"] == previous["run"] and
                receipt["definition_sha256"] == previous["definition_sha256"], "BUILD_PREVIOUS_RECEIPT_MISMATCH")
        for proof in (context, record):
            reference = {key: proof[key] for key in ("artifact", "path", "sha256", "size")}
            require(receipt["artifacts"].count(reference) == 1, "BUILD_PREVIOUS_RECEIPT_MISMATCH")
        accepted_conclusions = {"succeeded": {"success"}, "duplicate": {"success"},
                                "failed": {"failure", "timed_out", "startup_failure", "action_required"},
                                "cancelled": {"cancelled", "failure"}}
        require(meta["conclusion"] in accepted_conclusions.get(receipt["status"], set()),
                "BUILD_PREVIOUS_RESULT_UNCONFIRMED")
        if value["decision"] == "duplicate":
            require(receipt["status"] == "duplicate" and receipt["duplicate_of"] == value["owner"],
                    "BUILD_DUPLICATE_CHAIN_INVALID")
        else:
            require(receipt["status"] != "duplicate" and receipt["duplicate_of"] is None,
                    "BUILD_DUPLICATE_CHAIN_INVALID")
        self.processed_cache[key] = value
        return value

    def owner(self, value):
        owner = value["owner"]
        if value["decision"] == "duplicate":
            original = self.processed(self.metadata(owner["run_id"], owner["attempt"]))
            require(original is not None and original["decision"] == "owner" and original["owner"] == owner,
                    "BUILD_DUPLICATE_CHAIN_INVALID")
        if owner["run_id"] == self.current["run"]["run_id"]:
            # 显式 Re-run all jobs 可创建当前 run 的新 attempt；旧 duplicate
            # 不得让原处理者反过来认领自己，或把旧 attempt 混入本次证据。
            require(owner["attempt"] < self.current["run"]["attempt"], "BUILD_DUPLICATE_CHAIN_INVALID")
            return None
        latest = self.metadata(owner["run_id"])
        require(latest["run_attempt"] >= owner["attempt"], "BUILD_PREVIOUS_RUN_INVALID")
        if latest["run_attempt"] != owner["attempt"]:
            current_owner = self.processed(latest)
            require(current_owner is not None and current_owner["decision"] == "owner",
                    "BUILD_PREVIOUS_RESULT_UNCONFIRMED")
            owner = current_owner["owner"]
        return owner


def deduplicate(request, github, event_name="workflow_dispatch"):
    validate_request(request)
    if event_name == "push":
        require(request["request_id"] == "push-" + str(request["run"]["run_id"]) and
                request["build_mode"] == "verify", "BUILD_PUSH_IDENTITY_INVALID")
        return None
    require(event_name == "workflow_dispatch", "BUILD_EVENT_UNSUPPORTED")
    prefix = "PowerX request=" + request["request_id"] + " "
    runs = github.pages("/repos/" + REPOSITORY + "/actions/workflows/publish-container.yml/runs?event=workflow_dispatch",
                        "workflow_runs")
    peers = [run for run in runs if type(run.get("id")) is int and
             run["id"] != request["run"]["run_id"] and run.get("display_title", "").startswith(prefix)]
    reader, owners = RequestEvidence(request, github), {}
    for peer in sorted(peers, key=lambda run: run["id"]):
        value = reader.processed(reader.metadata(peer["id"]))
        if value is not None:
            owner = reader.owner(value)
            if owner is not None:
                owners[(owner["run_id"], owner["attempt"])] = owner
    require(len(owners) <= 1, "BUILD_REQUEST_OWNERSHIP_CONFLICT")
    return next(iter(owners.values()), None)


def tool_versions():
    versions = {"python": sys.version.split()[0]}
    for name, command in {"go": ["go", "version"], "docker": ["docker", "--version"],
                          "buildx": ["docker", "buildx", "version"], "gh": ["gh", "--version"]}.items():
        try:
            result = subprocess.run(command, capture_output=True, text=True, timeout=10, check=False)
            if result.returncode == 0:
                versions[name] = result.stdout.splitlines()[0][:200]
        except (OSError, subprocess.TimeoutExpired, IndexError):
            pass
    return versions


def image_environment(pair, template, source_sha):
    require(pair.get("source_sha") == source_sha and upgrade_release.SHA.fullmatch(source_sha),
            "BUILD_SOURCE_SHA_MISMATCH")
    for role, variable in (("backend", "POWERX_BACKEND_IMAGE"), ("web_admin", "POWERX_WEB_IMAGE")):
        image = pair["images"][role]
        repository = "ghcr.io/" + upgrade_release.REPOSITORIES[role]
        require(image.get("repository") == repository and
                upgrade_release.DIGEST.fullmatch(image.get("index_digest", "")) and
                set(image.get("platforms", {})) == {"linux/amd64", "linux/arm64"},
                "BUILD_IMAGE_PAIR_INVALID")
        old = variable + "=" + repository + ":REPLACE_WITH_PUBLISHED_TAG"
        require(template.count(old) == 1, "BUILD_ENVIRONMENT_TEMPLATE_INVALID")
        template = template.replace(old, variable + "=" + repository + "@" + image["index_digest"])
    require("REPLACE_WITH_PUBLISHED_TAG" not in template, "BUILD_ENVIRONMENT_TEMPLATE_INVALID")
    return template


def phase_evidence(name, result, env, steps, backend="", web=""):
    require(name in PHASES and result in RESULTS, "BUILD_PHASE_INVALID")
    image_digests = {}
    for role, value in {"backend": backend, "web_admin": web}.items():
        if value:
            require(upgrade_release.DIGEST.fullmatch(value), "BUILD_IMAGE_DIGEST_INVALID")
            image_digests[role] = value
    metadata = {}
    for role, key in {"backend": "BUILD_BACKEND_METADATA", "web_admin": "BUILD_WEB_METADATA"}.items():
        if env.get(key):
            require(len(env[key]) <= 2**20, "BUILD_METADATA_TOO_LARGE")
            raw = loads(env[key])
            require(isinstance(raw, dict), "BUILD_METADATA_INVALID")
            metadata[role] = {key: value for key, value in raw.items() if key in {
                "buildx.build.ref", "buildx.build.provenance", "containerimage.digest", "image.name"}}
    actions = sorted(set(re.findall(r"^\s+- uses: ([^\s]+)", (ROOT / WORKFLOW).read_text(), re.MULTILINE)))
    error = env.get("BUILD_PHASE_ERROR") or None
    require(error is None or re.fullmatch(r"(BUILD|UPGRADE)_[A-Z0-9_]+", error), "BUILD_PHASE_ERROR_INVALID")
    return {"schema_version": "powerx.build-phase/v1", "phase": name, "result": result, "error_code": error,
            "run": identity(env), "tools": tool_versions(), "image_digests": image_digests,
            "action_refs": actions, "build_metadata": metadata,
            "steps": {key: value.get("conclusion", "skipped") for key, value in steps.items()
                      if value.get("conclusion") in RESULTS}}


def file_reference(path, evidence_root, request):
    relative = path.relative_to(evidence_root)
    require(len(relative.parts) == 2, "BUILD_ARTIFACT_LAYOUT_INVALID")
    artifact = relative.parts[0]
    suffix = "-" + str(request["run"]["run_id"]) + "-" + str(request["run"]["attempt"])
    require(artifact.endswith(suffix) or suffix + "-" in artifact, "BUILD_ARTIFACT_ATTEMPT_MISMATCH")
    return {"artifact": artifact, "path": path.name, "sha256": digest(path.read_bytes()), "size": path.stat().st_size}


def make_receipt(evidence_root, env, jobs):
    root = Path(evidence_root)
    run = identity(env)
    requests = list(root.rglob("request-context.json"))
    require(len(requests) <= 1, "BUILD_REQUEST_EVIDENCE_DUPLICATE")
    request = validate_request(read(requests[0])) if requests else None
    if request:
        require(request["run"] == run and request["definition_sha256"] == digest((ROOT / WORKFLOW).read_bytes()),
                "BUILD_REQUEST_RUN_MISMATCH")
    receipt = {"schema_version": RECEIPT_SCHEMA, "run": run, "request": request,
               "status": "rejected" if request is None else "failed", "deployable": False,
               "duplicate_of": None, "phases": {}, "images": {}, "artifacts": [],
               "publication": None, "error_code": None,
               "definition_sha256": digest((ROOT / WORKFLOW).read_bytes())}
    for name, job in jobs.items():
        require(job.get("result") in RESULTS, "BUILD_JOB_RESULT_INVALID")
        receipt["phases"][name] = {"result": job["result"]}
    evidence = {}
    for path in sorted(root.rglob("phase-*.json")):
        phase = read(path)
        name = phase.get("phase")
        require(name in PHASES and name not in evidence and phase.get("run") == run and
                phase.get("result") in RESULTS and phase.get("schema_version") == "powerx.build-phase/v1",
                "BUILD_PHASE_EVIDENCE_INVALID")
        evidence[name] = phase
    # 所有阶段均保留工具和步骤结果，失败后的部分 digest 也是诊断证据。
    receipt["phase_evidence"] = evidence
    receipt["images"] = evidence.get("images", {}).get("image_digests", {})
    receipt["error_code"] = next((phase["error_code"] for phase in evidence.values() if phase.get("error_code")), None)
    if not request:
        receipt["error_code"] = receipt["error_code"] or "BUILD_PREFLIGHT_REJECTED"
        return receipt
    for path in sorted(root.rglob("*")):
        if path.is_file():
            receipt["artifacts"].append(file_reference(path, root, request))
    duplicate = jobs.get("preflight", {}).get("outputs", {}).get("duplicate_of", "")
    decisions = list(root.rglob("request-decision.json"))
    require(len(decisions) <= 1, "BUILD_DECISION_EVIDENCE_DUPLICATE")
    if jobs.get("preflight", {}).get("result") == "success":
        require(len(decisions) == 1, "BUILD_DECISION_EVIDENCE_MISSING")
        expected = decision_record(request, loads(duplicate) if duplicate else None)
        require(validate_decision(read(decisions[0]), request) == expected, "BUILD_DECISION_REQUEST_MISMATCH")
    if duplicate:
        require(jobs["preflight"]["result"] == "success", "BUILD_DUPLICATE_UNCONFIRMED")
        receipt["duplicate_of"] = loads(duplicate)
        require(receipt["duplicate_of"].get("input_sha256") == request["input_sha256"],
                "BUILD_DUPLICATE_UNCONFIRMED")
        receipt["status"] = "duplicate"
        return receipt
    if any(job["result"] == "cancelled" for job in jobs.values()):
        receipt["status"] = "cancelled"
        receipt["error_code"] = receipt["error_code"] or "BUILD_CANCELLED"
        return receipt
    if any(job["result"] == "failure" for job in jobs.values()):
        receipt["error_code"] = receipt["error_code"] or "BUILD_PHASE_FAILED"
        return receipt
    required = ["preflight", "checks"]
    if request["build_mode"] != "verify":
        required += ["images", "installed-upgrade", "upgrade-metadata"]
    if request["build_mode"] == "release":
        required += ["bundle-release"]
    if not all(jobs.get(name, {}).get("result") == "success" for name in required):
        receipt["error_code"] = "BUILD_PHASE_INCOMPLETE"
        return receipt
    required_evidence = ["preflight", "checks"]
    if request["build_mode"] != "verify":
        required_evidence += ["images", "upgrade-amd64", "upgrade-arm64", "metadata"]
    if request["build_mode"] == "release":
        required_evidence += ["release"]
    require(all(evidence.get(name, {}).get("result") == "success" for name in required_evidence),
            "BUILD_PHASE_EVIDENCE_MISSING")
    require({"python", "go"} <= set(evidence["checks"].get("tools", {})), "BUILD_TOOL_EVIDENCE_MISSING")
    if request["build_mode"] != "verify":
        require({"python", "docker", "buildx"} <= set(evidence["images"].get("tools", {})),
                "BUILD_TOOL_EVIDENCE_MISSING")
        pairs, manifests = list(root.rglob("target-pair.json")), list(root.rglob("powerx-upgrade-manifest.json"))
        require(len(pairs) == len(manifests) == 1, "BUILD_RELEASE_EVIDENCE_MISSING")
        pair, manifest = read(pairs[0]), read(manifests[0])
        reports = [{"report": read(path), "artifact": path.name, "sha256": digest(path.read_bytes())}
                   for path in sorted(root.rglob("upgrade-report-*.json"))]
        expected = upgrade_release.build_manifest(pair, read(upgrade_release.BASELINE), reports,
            "https://github.com/" + REPOSITORY + "/actions/runs/" + str(run["run_id"]), request["channel"])
        require(manifest == expected and pair["source_sha"] == request["actual_source_sha"] and
                set(manifest["verification"]["tested_upgrade_platforms"]) == {"linux/amd64", "linux/arm64"},
                "BUILD_RELEASE_EVIDENCE_MISMATCH")
        require(all(set(image["platforms"]) == {"linux/amd64", "linux/arm64"} for image in pair["images"].values()),
                "BUILD_PLATFORMS_INCOMPLETE")
        require(all(evidence["images"]["image_digests"].get(role) == pair["images"][role]["index_digest"]
                    for role in upgrade_release.REPOSITORIES), "BUILD_IMAGE_PAIR_MISMATCH")
        receipt["images"] = pair["images"]
        for filename in ("powerx-docker.tar.gz", "powerx-upgrade-manifest.json"):
            files, sums = list(root.rglob(filename)), list(root.rglob(filename + ".sha256"))
            require(len(files) == len(sums) == 1 and
                    sums[0].read_text().strip() == digest(files[0].read_bytes()) + "  " + filename,
                    "BUILD_CHECKSUM_MISMATCH")
    if request["build_mode"] == "release":
        publications = list(root.rglob("publication.json"))
        require(len(publications) == 1, "BUILD_PUBLICATION_MISSING")
        publication = read(publications[0])
        require(publication.get("source_sha") == request["actual_source_sha"] and publication.get("published") is True,
                "BUILD_PUBLICATION_UNCONFIRMED")
        tag = ("docker-dev-" if request["channel"] == "development" else "docker-release-") + request["actual_source_sha"][:12]
        require(publication.get("tag") == tag and
                publication.get("url") == "https://github.com/" + REPOSITORY + "/releases/tag/" + tag,
                "BUILD_PUBLICATION_UNCONFIRMED")
        for name in ("powerx-docker.tar.gz", "powerx-docker.tar.gz.sha256",
                     "powerx-upgrade-manifest.json", "powerx-upgrade-manifest.json.sha256"):
            files = list(root.rglob(name))
            asset = publication.get("assets", {}).get(name, {})
            require(len(files) == 1 and asset.get("sha256") == digest(files[0].read_bytes()) and
                    asset.get("size") == files[0].stat().st_size, "BUILD_PUBLICATION_ASSET_MISMATCH")
        receipt["publication"] = publication
        receipt["deployable"] = True
    receipt["status"] = "succeeded"
    return receipt


def github_outputs(values):
    path = os.environ.get("GITHUB_OUTPUT")
    if path:
        with open(path, "a") as stream:
            for key, value in values.items():
                require("\n" not in str(value) and "\r" not in str(value), "BUILD_OUTPUT_INVALID")
                stream.write(key + "=" + str(value) + "\n")


def checkout_identity():
    actual = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    dirty = subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=all"], text=True)
    require(not dirty.strip(), "BUILD_CHECKOUT_DIRTY")
    return actual


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    prepare_parser = commands.add_parser("prepare")
    prepare_parser.add_argument("--output", required=True)
    dedup_parser = commands.add_parser("deduplicate")
    dedup_parser.add_argument("--request", required=True)
    dedup_parser.add_argument("--output", required=True)
    phase = commands.add_parser("phase")
    phase.add_argument("--name", required=True)
    phase.add_argument("--result", required=True)
    phase.add_argument("--output", required=True)
    phase.add_argument("--backend", default="")
    phase.add_argument("--web", default="")
    receipt = commands.add_parser("receipt")
    receipt.add_argument("--evidence", required=True)
    receipt.add_argument("--output", required=True)
    environment = commands.add_parser("environment")
    environment.add_argument("--pair", required=True)
    environment.add_argument("--template", required=True)
    environment.add_argument("--output", required=True)
    environment.add_argument("--source-sha", required=True)
    args = parser.parse_args()
    if args.command == "prepare":
        actual = checkout_identity()
        result = prepare(read(os.environ["GITHUB_EVENT_PATH"]), os.environ, actual)
        write(args.output, result)
        github_outputs({"mode": result["build_mode"], "channel": result["channel"], "source_sha": actual,
                        "request_id": result["request_id"], "build_time": result["build_time"],
                        "version": ("dev-" if result["channel"] == "development" else "release-") + actual[:12]})
    elif args.command == "deduplicate":
        request = read(args.request)
        duplicate = deduplicate(request, GitHub(os.environ.get("GH_TOKEN")), os.environ["GITHUB_EVENT_NAME"])
        write(args.output, decision_record(request, duplicate))
        github_outputs({"fresh": "false" if duplicate else "true",
                        "duplicate_of": canonical(duplicate).decode() if duplicate else ""})
    elif args.command == "phase":
        write(args.output, phase_evidence(args.name, args.result, os.environ,
              loads(os.environ.get("BUILD_STEPS_JSON", "{}")), args.backend, args.web))
    elif args.command == "environment":
        Path(args.output).write_text(image_environment(read(args.pair), Path(args.template).read_text(), args.source_sha))
    else:
        try:
            result = make_receipt(args.evidence, os.environ, loads(os.environ["BUILD_JOBS_JSON"]))
        except Exception as error:
            code = str(error) if isinstance(error, ValueError) and str(error).startswith(("BUILD_", "UPGRADE_")) else "BUILD_RECEIPT_INVALID"
            run = identity(os.environ)
            accepted = None
            try:
                contexts = list(Path(args.evidence).rglob("request-context.json"))
                if len(contexts) == 1:
                    value = validate_request(read(contexts[0]))
                    if value["run"] == run:
                        accepted = value
            except Exception:
                pass
            result = {"schema_version": RECEIPT_SCHEMA, "run": run, "request": accepted,
                      "status": "failed" if accepted else "rejected", "deployable": False, "duplicate_of": None, "phases": {},
                      "phase_evidence": {}, "images": {}, "artifacts": [], "publication": None,
                      "definition_sha256": digest((ROOT / WORKFLOW).read_bytes()), "error_code": code}
        write(args.output, result)
        if result["status"] in {"failed", "rejected", "cancelled"}:
            sys.exit(1)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        code = str(error) if isinstance(error, ValueError) and str(error).startswith(("BUILD_", "UPGRADE_")) else "BUILD_CONTRACT_FAILED"
        github_outputs({"error_code": code})
        print(code, file=sys.stderr)
        sys.exit(1)
