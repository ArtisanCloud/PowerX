import unittest
import fnmatch
import re
import subprocess
import sys

import yaml
import build_contract


class PublishWorkflowTests(unittest.TestCase):
    def setUp(self):
        self.text = (build_contract.ROOT / build_contract.WORKFLOW).read_text()
        self.workflow = yaml.load(self.text, Loader=yaml.BaseLoader)
        self.jobs = self.workflow["jobs"]

    def test_dispatch_contract_and_request_serialization(self):
        inputs = self.workflow["on"]["workflow_dispatch"]["inputs"]
        self.assertEqual(set(inputs), build_contract.INPUTS)
        self.assertTrue(all(item["required"] == "true" for item in inputs.values()))
        self.assertEqual(set(inputs["build_mode"]["options"]), build_contract.MODES)
        self.assertEqual(self.workflow["concurrency"]["cancel-in-progress"], "false")
        self.assertIn("inputs.request_id", self.workflow["concurrency"]["group"])

    def test_verify_has_no_registry_or_release_write_permissions(self):
        self.assertEqual(self.workflow["permissions"], {"contents": "read"})
        self.assertNotIn("permissions", self.jobs["checks"])
        for name, job in self.jobs.items():
            if job.get("permissions", {}).get("packages") == "write":
                self.assertEqual(name, "images")
                self.assertIn("mode != 'verify'", job["if"])
                self.assertIn("checks", job["needs"])
            if job.get("permissions", {}).get("contents") == "write":
                self.assertEqual(name, "bundle-release")
                self.assertIn("mode == 'release'", job["if"])
        self.assertNotIn("--clobber", self.text)
        self.assertIn("inputs.build_mode || 'verify'", self.workflow["run-name"])

    def test_each_runner_installs_its_own_required_python_dependencies(self):
        images = self.jobs["images"]["steps"]
        setup = next(i for i, step in enumerate(images) if step.get("uses", "").startswith("actions/setup-python@"))
        install = next(i for i, step in enumerate(images) if "pip install PyYAML" in step.get("run", ""))
        imports = next(i for i, step in enumerate(images) if "import smoke, upgrade_smoke" in step.get("run", ""))
        login = next(i for i, step in enumerate(images) if step.get("uses", "").startswith("docker/login-action@"))
        self.assertLess(setup, install)
        self.assertLess(install, imports)
        self.assertLess(imports, login)
        self.assertTrue(any("pip install jsonschema" in step.get("run", "") for step in self.jobs["preflight"]["steps"]))

    def test_smoke_import_does_not_start_setup_or_docker(self):
        result = subprocess.run([sys.executable, "-c", "import smoke, upgrade_smoke; print('imports-only')"],
                                cwd=build_contract.ROOT / "deploy/powerx/container",
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "imports-only")

    def test_processing_decision_is_persisted_before_other_jobs(self):
        steps = self.jobs["preflight"]["steps"]
        claim = next(i for i, step in enumerate(steps) if step.get("id") == "claim")
        commit = next(i for i, step in enumerate(steps)
                      if step.get("with", {}).get("path") == "request-decision.json")
        self.assertLess(claim, commit)
        self.assertEqual(steps[commit]["with"]["if-no-files-found"], "error")
        self.assertIn("preflight", self.jobs["checks"]["needs"])

    def test_source_pair_and_attempt_identity_are_preserved(self):
        for job in self.jobs.values():
            for step in job["steps"]:
                if step.get("uses", "").startswith("actions/checkout@"):
                    self.assertEqual(step["with"]["ref"], "${{ github.sha }}")
                    self.assertEqual(step["with"]["persist-credentials"], "false")
                if step.get("uses", "").startswith("actions/upload-artifact@"):
                    self.assertIn("github.run_id", step["with"]["name"])
                    self.assertIn("github.run_attempt", step["with"]["name"])
        images = self.jobs["images"]["steps"]
        for step in images:
            if step.get("uses", "").startswith("docker/build-push-action@"):
                self.assertIn("POWERX_SOURCE_SHA=", step["with"]["build-args"])
                self.assertIn("POWERX_VERSION=", step["with"]["build-args"])
                self.assertIn(":ci-", step["with"]["tags"])
                self.assertNotIn(":sha-", step["with"]["tags"])
        smoke = next(step for step in images if step.get("name", "").startswith("Fresh database"))
        self.assertIn("build_contract.py environment --pair target-pair.json", smoke["run"])

    def test_original_upgrade_migration_and_restore_gates_remain(self):
        matrix = self.jobs["installed-upgrade"]["strategy"]["matrix"]["include"]
        self.assertEqual({item["platform"] for item in matrix}, {"linux/amd64", "linux/arm64"})
        self.assertIn("TestInstalledCoreUpgradeMigratesLegacyRootBaselineWithNewIdentity", self.text)
        self.assertIn("TestPostgresMigration", self.text)
        self.assertIn("upgrade_smoke.py", self.text)
        self.assertIn("--require-all-platforms", self.text)
        receipt = self.jobs["build-receipt"]
        self.assertEqual(receipt["if"], "always()")
        self.assertTrue({"installed-upgrade", "upgrade-metadata", "bundle-release"} <= set(receipt["needs"]))
        self.assertIn("restore_rollback", build_contract.upgrade_release.REQUIRED_CHECKS)

    def test_receipt_download_cannot_mix_attempt_one_and_ten(self):
        for step in self.jobs["build-receipt"]["steps"]:
            pattern = step.get("with", {}).get("pattern")
            if pattern:
                pattern = pattern.replace("${{ github.run_id }}", "100").replace("${{ github.run_attempt }}", "1")
                self.assertFalse(fnmatch.fnmatch("powerx-build-request-100-10", pattern))
                self.assertFalse(fnmatch.fnmatch("powerx-upgrade-report-100-10-amd64", pattern))

    def test_shell_steps_parse(self):
        for job in self.jobs.values():
            for step in job["steps"]:
                if "run" in step:
                    script = re.sub(r"\$\{\{.*?\}\}", "CI_VALUE", step["run"])
                    result = subprocess.run(["bash", "-n"], input=script, text=True, capture_output=True)
                    self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
