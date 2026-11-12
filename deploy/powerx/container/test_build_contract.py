import copy
import io
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest
from unittest.mock import patch
import urllib.request
import zipfile

import jsonschema
import build_contract as contract
import test_upgrade_release
import upgrade_release


def environment(sha="a" * 40, run=100, attempt=1, ref="refs/heads/develop"):
    return {"GITHUB_REPOSITORY": contract.REPOSITORY, "GITHUB_SHA": sha, "GITHUB_REF": ref,
            "GITHUB_WORKFLOW_REF": contract.REPOSITORY + "/" + contract.WORKFLOW + "@" + ref,
            "GITHUB_WORKFLOW_SHA": sha, "GITHUB_EVENT_NAME": "workflow_dispatch",
            "GITHUB_RUN_ID": str(run), "GITHUB_RUN_ATTEMPT": str(attempt)}


def request(mode="build", env=None, request_id="xdocker-request-0001"):
    env = env or environment()
    return contract.prepare({"inputs": {"request_id": request_id, "expected_source_sha": env["GITHUB_SHA"],
                                        "build_mode": mode}}, env, env["GITHUB_SHA"])


def scenario(root, mode="build"):
    env = environment()
    ctx = request(mode, env)
    suffix = "-100-1"

    def store(artifact, name, data):
        target = root / (artifact + suffix) / name
        contract.write(target, data)
        return target

    store("powerx-build-request", "request-context.json", ctx)
    jobs = {name: {"result": "success" if name in ("preflight", "checks") else "skipped"}
            for name in ("preflight", "checks", "images", "installed-upgrade", "upgrade-metadata", "bundle-release")}
    phases = ["preflight", "checks"]
    pair = None
    if mode != "verify":
        for name in ("images", "installed-upgrade", "upgrade-metadata"):
            jobs[name]["result"] = "success"
        phases += ["images", "upgrade-amd64", "upgrade-arm64", "metadata"]
        pair, baseline, reports = test_upgrade_release.UpgradeReleaseTests().fixture()
        second = copy.deepcopy(reports[0])
        second["report"]["platform"] = "linux/arm64"
        second["artifact"] = "upgrade-report-linux-arm64.json"
        for role in upgrade_release.REPOSITORIES:
            second["report"]["source_images"][role] = baseline["images"][role]["platforms"]["linux/arm64"]["digest"]
        reports.append(second)
        for report in reports:
            arch = report["report"]["platform"].split("/")[1]
            path = root / ("powerx-upgrade-report" + suffix + "-" + arch) / report["artifact"]
            contract.write(path, report["report"])
            report["sha256"] = contract.digest(path.read_bytes())
        store("powerx-target-pair", "target-pair.json", pair)
        manifest = upgrade_release.build_manifest(pair, baseline, reports,
                "https://github.com/ArtisanCloud/PowerX/actions/runs/100", "development")
        manifest_path = store("powerx-docker", "powerx-upgrade-manifest.json", manifest)
        bundle = manifest_path.with_name("powerx-docker.tar.gz")
        bundle.write_bytes(b"isolated deployment bundle fixture")
        for path in (bundle, manifest_path):
            path.with_name(path.name + ".sha256").write_text(contract.digest(path.read_bytes()) + "  " + path.name + "\n")
    if mode == "release":
        jobs["bundle-release"]["result"] = "success"
        phases.append("release")
        directory = root / ("powerx-docker" + suffix)
        assets = {path.name: {"id": index + 1, "sha256": contract.digest(path.read_bytes()),
                             "size": path.stat().st_size} for index, path in enumerate(sorted(directory.iterdir()))}
        store("powerx-publication", "publication.json",
              {"source_sha": env["GITHUB_SHA"], "release_id": 123, "tag": "docker-dev-" + "a" * 12,
               "url": "https://github.com/ArtisanCloud/PowerX/releases/tag/docker-dev-" + "a" * 12,
               "published": True, "reused": False, "assets": assets})
    with patch.object(contract, "tool_versions", return_value={"python": "3.13.0", "buildx": "fixture",
                                                             "docker": "fixture", "go": "fixture"}):
        for name in phases:
            value = contract.phase_evidence(name, "success", env, {})
            if name == "images":
                value["image_digests"] = {role: image["index_digest"] for role, image in pair["images"].items()}
            store("powerx-phase-" + name, "phase-" + name + ".json", value)
    return env, jobs


class RequestTests(unittest.TestCase):
    def test_supported_ref_and_mode_matrix(self):
        for ref in contract.CHANNELS:
            for mode in contract.MODES:
                with self.subTest(ref=ref, mode=mode):
                    ctx = request(mode, environment(ref=ref))
                    self.assertEqual(ctx["channel"], contract.CHANNELS[ref])
                    self.assertEqual(contract.validate_request(ctx), ctx)

    def test_bad_inputs_fail_before_build(self):
        cases = [
            ({"inputs": {}}, environment(), "a" * 40, "BUILD_INPUT_KEYS_INVALID"),
            ({"inputs": {"request_id": "request-0001", "expected_source_sha": "a" * 40, "build_mode": "ignored"}},
             environment(), "a" * 40, "BUILD_MODE_UNSUPPORTED"),
        ]
        good = {"inputs": {key: request()[key] for key in contract.INPUTS}}
        for key in contract.INPUTS:
            broken = copy.deepcopy(good)
            del broken["inputs"][key]
            cases.append((broken, environment(), "a" * 40, "BUILD_INPUT_KEYS_INVALID"))
        broken = copy.deepcopy(good)
        broken["inputs"]["url"] = "https://untrusted.invalid"
        cases.append((broken, environment(), "a" * 40, "BUILD_INPUT_KEYS_INVALID"))
        cases += [(good, environment(), "b" * 40, "BUILD_SOURCE_SHA_MISMATCH"),
                  (good, environment(sha="b" * 40), "b" * 40, "BUILD_SOURCE_SHA_MISMATCH"),
                  (good, environment(ref="refs/tags/v1"), "a" * 40, "BUILD_REF_UNSUPPORTED")]
        for event, env, actual, error in cases:
            with self.subTest(error=error):
                with self.assertRaisesRegex(ValueError, error):
                    contract.prepare(event, env, actual)

    def test_push_is_explicit_release_and_rerun_keeps_request_identity(self):
        env = environment()
        env["GITHUB_EVENT_NAME"] = "push"
        event = {"ref": env["GITHUB_REF"], "after": env["GITHUB_SHA"]}
        original = contract.prepare(event, env, env["GITHUB_SHA"])
        env["GITHUB_RUN_ATTEMPT"] = "2"
        retry = contract.prepare(event, env, env["GITHUB_SHA"])
        self.assertEqual(original["input_sha256"], retry["input_sha256"])
        self.assertNotEqual(original["run"]["attempt"], retry["run"]["attempt"])
        self.assertEqual(retry["build_mode"], "release")

    def test_unknown_json_and_request_fields_rejected(self):
        with self.assertRaisesRegex(ValueError, "BUILD_DUPLICATE_JSON_KEY"):
            contract.loads('{"build_mode":"verify","build_mode":"release"}')
        ctx = request()
        ctx["endpoint"] = "/arbitrary"
        with self.assertRaisesRegex(ValueError, "BUILD_REQUEST_FIELDS_INVALID"):
            contract.validate_request(ctx)

    def test_redirect_does_not_forward_repository_token(self):
        original = urllib.request.Request("https://api.github.com/file", headers={"Authorization": "Bearer private"})
        redirected = contract.SafeRedirect().redirect_request(original, None, 302, "", {},
                                                              "https://objects.githubusercontent.com/file")
        self.assertIsNone(redirected.get_header("Authorization"))

    def test_dirty_checkout_is_rejected(self):
        with patch.object(contract.subprocess, "check_output", side_effect=["a" * 40 + "\n", " M backend/go.mod\n"]):
            with self.assertRaisesRegex(ValueError, "BUILD_CHECKOUT_DIRTY"):
                contract.checkout_identity()

    def test_github_array_lists_and_incomplete_pages(self):
        api = contract.GitHub("test-token")
        with patch.object(api, "request", return_value=[{"id": 1}]):
            self.assertEqual(api.pages("/repos/ArtisanCloud/PowerX/releases", "releases"), [{"id": 1}])
        with patch.object(api, "request", return_value={"total_count": 2, "artifacts": [{"id": 1}]}):
            with self.assertRaisesRegex(ValueError, "BUILD_GITHUB_LIST_INCOMPLETE"):
                api.pages("/repos/ArtisanCloud/PowerX/actions/runs/1/artifacts", "artifacts")


class DuplicateTests(unittest.TestCase):
    def fake_api(self, previous):
        class API:
            def pages(self, path, field):
                if field == "workflow_runs":
                    return [{"id": 99, "display_title": "PowerX request=xdocker-request-0001 mode=build sha=" + "a" * 40,
                             "path": contract.WORKFLOW, "repository": {"full_name": contract.REPOSITORY},
                             "head_sha": "a" * 40, "head_branch": "develop", "run_attempt": 1}]
                return [{"name": "powerx-build-request-99-1", "id": 1, "expired": False}]

            def request(self, *args, **kwargs):
                data = io.BytesIO()
                with zipfile.ZipFile(data, "w") as archive:
                    archive.writestr("request-context.json", json.dumps(previous))
                return data.getvalue()
        return API()

    def test_same_request_links_existing_run_and_changed_payload_conflicts(self):
        previous = request(env=environment(run=99))
        api = self.fake_api(previous)
        self.assertEqual(contract.deduplicate(request(), api)["run_id"], 99)
        for mode in ("verify", "release"):
            with self.assertRaisesRegex(ValueError, "BUILD_REQUEST_ID_CONFLICT"):
                contract.deduplicate(request(mode), api)

    def test_missing_or_expired_request_artifact_blocks_replay(self):
        api = self.fake_api(request(env=environment(run=99)))
        original = api.pages
        api.pages = lambda path, field: [] if field == "artifacts" else original(path, field)
        with self.assertRaisesRegex(ValueError, "BUILD_PREVIOUS_REQUEST_UNCONFIRMED"):
            contract.deduplicate(request(), api)


class ReceiptTests(unittest.TestCase):
    def schema(self):
        return contract.read(contract.ROOT / "docs/contracts/powerx-build-receipt.schema.json")

    def test_all_modes_schema_and_deployability(self):
        for mode in ("verify", "build", "release"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temp:
                env, jobs = scenario(Path(temp), mode)
                receipt = contract.make_receipt(temp, env, jobs)
                jsonschema.validate(receipt, self.schema())
                self.assertEqual(receipt["status"], "succeeded")
                self.assertEqual(receipt["deployable"], mode == "release")
                if mode == "verify":
                    receipt["deployable"] = True
                    with self.assertRaises(jsonschema.ValidationError):
                        jsonschema.validate(receipt, self.schema())

    def test_failure_cancel_and_duplicate_never_authorize_deployment(self):
        for result in ("failure", "cancelled"):
            with tempfile.TemporaryDirectory() as temp:
                env, jobs = scenario(Path(temp))
                jobs["installed-upgrade"]["result"] = result
                receipt = contract.make_receipt(temp, env, jobs)
                jsonschema.validate(receipt, self.schema())
                self.assertEqual(receipt["status"], "cancelled" if result == "cancelled" else "failed")
                self.assertFalse(receipt["deployable"])
        with tempfile.TemporaryDirectory() as temp:
            env, jobs = scenario(Path(temp), "verify")
            jobs["preflight"]["outputs"] = {"duplicate_of": json.dumps(
                {"run_id": 99, "attempt": 1, "input_sha256": request("verify")["input_sha256"]})}
            jobs["checks"]["result"] = "skipped"
            receipt = contract.make_receipt(temp, env, jobs)
            jsonschema.validate(receipt, self.schema())
            self.assertEqual(receipt["status"], "duplicate")
            self.assertFalse(receipt["deployable"])

    def test_missing_tampered_and_cross_attempt_evidence_rejected(self):
        for case in ("report", "checksum", "digest", "attempt", "publication"):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                env, jobs = scenario(root, "release")
                if case == "report":
                    next(root.rglob("upgrade-report-linux-arm64.json")).unlink()
                elif case == "checksum":
                    next(root.rglob("powerx-docker.tar.gz")).write_bytes(b"tampered")
                elif case == "digest":
                    path = next(root.rglob("phase-images.json"))
                    value = contract.read(path)
                    value["image_digests"]["backend"] = "sha256:" + "f" * 64
                    contract.write(path, value)
                elif case == "attempt":
                    path = next(root.rglob("request-context.json"))
                    path.parent.rename(path.parent.with_name("powerx-build-request-100-2"))
                else:
                    next(root.rglob("publication.json")).unlink()
                with self.assertRaises(ValueError):
                    contract.make_receipt(root, env, jobs)

    def test_smoke_and_bundle_use_exact_pair_digests(self):
        pair, _, _ = test_upgrade_release.UpgradeReleaseTests().fixture()
        text = (contract.ROOT / "deploy/powerx/container/.env.example").read_text()
        result = contract.image_environment(pair, text, pair["source_sha"])
        self.assertNotIn("REPLACE_WITH_PUBLISHED_TAG", result)
        for role, image in pair["images"].items():
            self.assertIn(image["repository"] + "@" + image["index_digest"], result)
        with self.assertRaisesRegex(ValueError, "BUILD_SOURCE_SHA_MISMATCH"):
            contract.image_environment(pair, text, "d" * 40)

    def test_manifest_cli_requires_both_upgrade_platforms(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            scenario(root)
            command = [sys.executable, str(contract.ROOT / "deploy/powerx/container/upgrade_release.py"),
                       "manifest", "--pair", str(next(root.rglob("target-pair.json"))),
                       "--reports-dir", str(root), "--ci-run-url",
                       "https://github.com/ArtisanCloud/PowerX/actions/runs/100",
                       "--channel", "development", "--require-all-platforms", "--output", str(root / "result.json")]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 0)
            next(root.rglob("upgrade-report-linux-arm64.json")).unlink()
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("UPGRADE_ACCEPTANCE_PLATFORMS_INCOMPLETE", result.stderr)


class ExampleTests(unittest.TestCase):
    def test_schema_positive_negative_and_semantic_examples(self):
        directory = contract.ROOT / "docs/contracts/examples/powerx-build-receipt-v1"
        schema = contract.read(contract.ROOT / "docs/contracts/powerx-build-receipt.schema.json")
        examples = list(directory.glob("*.json"))
        self.assertGreaterEqual(len(examples), 9)
        for path in examples:
            with self.subTest(path=path.name):
                value = contract.read(path)
                if path.name.startswith("invalid-"):
                    with self.assertRaises(jsonschema.ValidationError):
                        jsonschema.validate(value, schema)
                else:
                    jsonschema.validate(value, schema)
                    if path.name == "semantic-sha-mismatch.json":
                        with self.assertRaises(ValueError):
                            contract.validate_request(value["request"])


if __name__ == "__main__":
    unittest.main()
