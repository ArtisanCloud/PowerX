import copy
from pathlib import Path
import tempfile
import unittest

import build_contract as contract
import release_publish
from test_build_contract import request, scenario


class MemoryGitHub:
    """模拟实际草稿、不可变上传和响应丢失；不访问 Registry/GitHub。"""
    def __init__(self):
        self.release = None
        self.assets = {}
        self.blobs = {}
        self.writes = []
        self.fail_upload_after = None
        self.fail_publish_response = False
        self.tag_override = None

    def pages(self, path, field):
        if field == "releases":
            return [copy.deepcopy(self.release)] if self.release else []
        return list(copy.deepcopy(self.assets).values())

    def request(self, path, method="GET", data=None, **kwargs):
        if "/git/ref/tags/" in path:
            if self.tag_override:
                return {"object": {"type": "commit", "sha": self.tag_override}}
            if self.release and not self.release["draft"]:
                return {"object": {"type": "commit", "sha": self.release["target_commitish"]}}
            return None
        if method == "POST":
            self.writes.append("create")
            self.release = {"id": 50, **copy.deepcopy(data)}
            return copy.deepcopy(self.release)
        if method == "PATCH":
            self.writes.append("publish")
            self.release["draft"] = data["draft"]
            if self.fail_publish_response:
                self.fail_publish_response = False
                raise OSError("simulated response loss")
            return copy.deepcopy(self.release)
        if "/releases/assets/" in path:
            return self.blobs[int(path.rsplit("/", 1)[1])]
        if "/tags/" in path and (not self.release or self.release["draft"]):
            return None
        return copy.deepcopy(self.release)

    def upload(self, release_id, name, raw):
        if name in self.assets:
            raise AssertionError("must not overwrite")
        self.writes.append("upload:" + name)
        asset = {"id": len(self.assets) + 1, "name": name, "size": len(raw), "state": "uploaded"}
        self.assets[name] = asset
        self.blobs[asset["id"]] = raw
        if self.fail_upload_after == len(self.assets):
            raise OSError("simulated cancellation after server accepted asset")
        return copy.deepcopy(asset)


class ReleasePublishTests(unittest.TestCase):
    def fixture(self, root):
        scenario(root, "release")
        return request("release"), root / "powerx-docker-100-1"

    def test_publish_and_exact_repeat_are_idempotent(self):
        with tempfile.TemporaryDirectory() as temp:
            ctx, directory = self.fixture(Path(temp))
            api = MemoryGitHub()
            publication = release_publish.publish(ctx, directory, api)
            self.assertTrue(publication["published"])
            self.assertFalse(api.release["draft"])
            writes = list(api.writes)
            second = release_publish.publish(ctx, directory, api)
            self.assertTrue(second["reused"])
            self.assertEqual(api.writes, writes)

    def test_partial_upload_and_publish_response_loss_recover_by_readback(self):
        for boundary in ("upload", "publication"):
            with self.subTest(boundary=boundary), tempfile.TemporaryDirectory() as temp:
                ctx, directory = self.fixture(Path(temp))
                api = MemoryGitHub()
                if boundary == "upload":
                    api.fail_upload_after = 2
                else:
                    api.fail_publish_response = True
                with self.assertRaises(OSError):
                    release_publish.publish(ctx, directory, api)
                api.fail_upload_after = None
                result = release_publish.publish(ctx, directory, api)
                self.assertTrue(result["published"])
                self.assertEqual(len(api.assets), 4)
                self.assertEqual(sum(item == "create" for item in api.writes), 1)
                self.assertTrue(all(api.writes.count("upload:" + name) == 1 for name in release_publish.ASSETS))

    def test_rebuild_changed_image_digest_cannot_replace_original_release(self):
        for draft in (False, True):
            with self.subTest(draft=draft), tempfile.TemporaryDirectory() as temp:
                ctx, directory = self.fixture(Path(temp))
                api = MemoryGitHub()
                release_publish.publish(ctx, directory, api)
                api.release["draft"] = draft
                before_assets, before_blobs, writes = copy.deepcopy(api.assets), copy.deepcopy(api.blobs), list(api.writes)
                manifest_path = directory / "powerx-upgrade-manifest.json"
                manifest = contract.read(manifest_path)
                manifest["images"]["backend"]["index_digest"] = "sha256:" + "f" * 64
                contract.write(manifest_path, manifest)
                (directory / (manifest_path.name + ".sha256")).write_text(
                    contract.digest(manifest_path.read_bytes()) + "  " + manifest_path.name + "\n")
                with self.assertRaisesRegex(ValueError, "BUILD_RELEASE_(REBUILD|ASSET)_CONFLICT"):
                    release_publish.publish(ctx, directory, api)
                self.assertEqual(api.assets, before_assets)
                self.assertEqual(api.blobs, before_blobs)
                self.assertEqual(api.writes, writes)

    def test_published_missing_asset_is_not_silently_repaired(self):
        with tempfile.TemporaryDirectory() as temp:
            ctx, directory = self.fixture(Path(temp))
            api = MemoryGitHub()
            release_publish.publish(ctx, directory, api)
            del api.assets["powerx-docker.tar.gz.sha256"]
            writes = list(api.writes)
            with self.assertRaisesRegex(ValueError, "BUILD_RELEASE_PUBLISHED_INCOMPLETE"):
                release_publish.publish(ctx, directory, api)
            self.assertEqual(writes, api.writes)

    def test_build_and_verify_modes_cannot_publish(self):
        with tempfile.TemporaryDirectory() as temp:
            for mode in ("build", "verify"):
                with self.assertRaisesRegex(ValueError, "BUILD_RELEASE_MODE_REQUIRED"):
                    release_publish.publish(request(mode), temp, MemoryGitHub())

    def test_existing_tag_with_wrong_source_is_rejected_before_writes(self):
        with tempfile.TemporaryDirectory() as temp:
            ctx, directory = self.fixture(Path(temp))
            api = MemoryGitHub()
            api.tag_override = "b" * 40
            with self.assertRaisesRegex(ValueError, "BUILD_RELEASE_TAG_SOURCE_MISMATCH"):
                release_publish.publish(ctx, directory, api)
            self.assertEqual(api.writes, [])


if __name__ == "__main__":
    unittest.main()
