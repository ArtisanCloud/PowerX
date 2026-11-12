"""固定发行身份：仅补同一草稿缺失资产，绝不覆盖既有 Release 资产。"""
import argparse
import json
import os
from pathlib import Path
import re
import sys
import urllib.parse

from build_contract import GitHub, REPOSITORY, canonical, digest, github_outputs, read, require, validate_request, write

ASSETS = ("powerx-docker.tar.gz", "powerx-docker.tar.gz.sha256",
          "powerx-upgrade-manifest.json", "powerx-upgrade-manifest.json.sha256")
MARKER = re.compile(r"<!-- powerx-release-identity:([^\n]+) -->")


def verify_tag(github, tag, source_sha, required=False):
    value = github.request("/repos/" + REPOSITORY + "/git/ref/tags/" + tag)
    if value is None:
        require(not required, "BUILD_RELEASE_TAG_UNCONFIRMED")
        return
    target = value.get("object", {})
    for _ in range(4):
        if target.get("type") != "tag":
            break
        require(re.fullmatch(r"[a-f0-9]{40}", target.get("sha", "")), "BUILD_RELEASE_TAG_INVALID")
        annotated = github.request("/repos/" + REPOSITORY + "/git/tags/" + target["sha"])
        require(isinstance(annotated, dict), "BUILD_RELEASE_TAG_INVALID")
        target = annotated.get("object", {})
    require(target.get("type") == "commit" and target.get("sha") == source_sha,
            "BUILD_RELEASE_TAG_SOURCE_MISMATCH")


def candidate(request, directory):
    validate_request(request)
    require(request["build_mode"] == "release", "BUILD_RELEASE_MODE_REQUIRED")
    directory = Path(directory)
    files = {name: (directory / name).read_bytes() for name in ASSETS}
    require(all(len(raw) <= 16 * 2**20 for raw in files.values()), "BUILD_RELEASE_ASSET_TOO_LARGE")
    for name in ("powerx-docker.tar.gz", "powerx-upgrade-manifest.json"):
        require(files[name + ".sha256"].decode().strip() == digest(files[name]) + "  " + name,
                "BUILD_RELEASE_CHECKSUM_MISMATCH")
    manifest = read(directory / "powerx-upgrade-manifest.json")
    require(manifest.get("schema_version") == "powerx.upgrade-manifest/v1" and
            manifest.get("source_sha") == request["actual_source_sha"] and
            manifest.get("release_id") == "sha-" + request["actual_source_sha"] and
            manifest.get("channel") == request["channel"] and manifest.get("executable") is True and
            manifest.get("verification", {}).get("status") == "passed" and
            manifest["verification"].get("ci_run_url") ==
            "https://github.com/" + REPOSITORY + "/actions/runs/" + str(request["run"]["run_id"]) and
            set(manifest["verification"].get("tested_upgrade_platforms", [])) == {"linux/amd64", "linux/arm64"},
            "BUILD_RELEASE_MANIFEST_INVALID")
    identity = {"source_sha": request["actual_source_sha"], "channel": request["channel"],
                "assets": {name: {"sha256": digest(raw), "size": len(raw)} for name, raw in files.items()}}
    return identity, files


def publish(request, directory, github):
    frozen, files = candidate(request, directory)
    prefix = "docker-dev-" if request["channel"] == "development" else "docker-release-"
    tag = prefix + request["actual_source_sha"][:12]
    base = "/repos/" + REPOSITORY + "/releases"
    verify_tag(github, tag, request["actual_source_sha"])
    release = github.request(base + "/tags/" + tag)
    if release is None:
        # API 的 tags 查询不保证列出草稿；从正式列表精确寻找，不把 404 当作可覆盖。
        matches = [item for item in github.pages(base, "releases") if item.get("tag_name") == tag]
        require(len(matches) <= 1, "BUILD_RELEASE_DUPLICATE")
        release = matches[0] if matches else None
    reused = release is not None and release.get("draft") is False
    body = ("Verified paired PowerX images. restore_required requires matching data and configuration restore.\n"
            "<!-- powerx-release-identity:" + canonical(frozen).decode() + " -->")
    if release is None:
        release = github.request(base, "POST", {"tag_name": tag, "target_commitish": request["actual_source_sha"],
            "name": "PowerX Docker " + request["actual_source_sha"][:12], "body": body, "draft": True,
            "prerelease": request["channel"] == "development"})
    require(isinstance(release, dict) and type(release.get("id")) is int and release["id"] > 0 and
            release.get("tag_name") == tag, "BUILD_RELEASE_IDENTITY_INVALID")
    release_id = release["id"]
    require(type(release.get("draft")) is bool, "BUILD_RELEASE_STATE_INVALID")
    require(release.get("prerelease") is (request["channel"] == "development"), "BUILD_RELEASE_CHANNEL_MISMATCH")
    if release["draft"]:
        match = MARKER.search(release.get("body", ""))
        require(match is not None and json.loads(match[1]) == frozen, "BUILD_RELEASE_REBUILD_CONFLICT")
    assets = github.pages(base + "/" + str(release_id) + "/assets", "assets")
    require(len({asset.get("name") for asset in assets}) == len(assets), "BUILD_RELEASE_DUPLICATE_ASSET")
    known = {asset.get("name"): asset for asset in assets}
    require(set(known) <= set(ASSETS), "BUILD_RELEASE_UNEXPECTED_ASSET")

    def verify_asset(name, asset):
        require(type(asset.get("id")) is int and asset["id"] > 0 and asset.get("state") == "uploaded" and
                asset.get("size") == frozen["assets"][name]["size"], "BUILD_RELEASE_ASSET_CONFLICT")
        raw = github.request("/repos/" + REPOSITORY + "/releases/assets/" + str(asset["id"]),
                             accept="application/octet-stream", limit=16 * 2**20)
        require(isinstance(raw, bytes) and digest(raw) == frozen["assets"][name]["sha256"],
                "BUILD_RELEASE_REBUILD_CONFLICT")
        return {"id": asset["id"], **frozen["assets"][name]}

    verified = {}
    # 所有已存在资产先比对，再补缺；不删除任何不匹配/未完成资产。
    for name, asset in known.items():
        verified[name] = verify_asset(name, asset)
    missing = set(ASSETS) - set(known)
    require(not missing or release["draft"], "BUILD_RELEASE_PUBLISHED_INCOMPLETE")
    for name in sorted(missing):
        verified[name] = verify_asset(name, github.upload(release_id, name, files[name]))
    if release["draft"]:
        release = github.request(base + "/" + str(release_id), "PATCH", {"draft": False})
    # 上传或发布响应丢失时本次失败；后续查询可用同一候选核对，不覆盖。
    confirmed = github.request(base + "/" + str(release_id))
    require(isinstance(confirmed, dict) and confirmed.get("draft") is False and confirmed.get("tag_name") == tag,
            "BUILD_RELEASE_PUBLICATION_UNCONFIRMED")
    verify_tag(github, tag, request["actual_source_sha"], required=True)
    return {"source_sha": request["actual_source_sha"], "release_id": release_id, "tag": tag,
            "url": "https://github.com/" + REPOSITORY + "/releases/tag/" + tag,
            "published": True, "reused": reused, "assets": verified}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--request", required=True)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    write(args.output, publish(read(args.request), args.directory, GitHub(os.environ.get("GH_TOKEN"))))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        code = str(error) if isinstance(error, ValueError) and str(error).startswith("BUILD_") else "BUILD_RELEASE_PUBLICATION_FAILED"
        github_outputs({"error_code": code})
        print(code, file=sys.stderr)
        sys.exit(1)
