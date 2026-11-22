#!/usr/bin/env python3
"""请求 PowerX 复查一批到期系统记录。默认预览，--apply 确认同一份预览。"""

import argparse
import http.client
import json
import os
import sys
import uuid
from urllib.parse import urlsplit


class APIError(RuntimeError):
    def __init__(self, status):
        super().__init__(f"PowerX HTTP {status}")
        self.status = status


def expiry_batch(origin, token, instance, revision, apply, emit):
    target = urlsplit(origin)
    if (target.username or target.password or target.query or target.fragment or
            not target.hostname or not token):
        raise ValueError("invalid_origin_or_missing_token")
    if target.scheme == "https":
        connection = http.client.HTTPSConnection
    elif target.scheme == "http" and target.hostname in ("localhost", "127.0.0.1", "::1"):
        connection = http.client.HTTPConnection
    else:
        raise ValueError("https_required")
    prefix = target.path.rstrip("/") or "/api/v1"
    route = prefix + "/admin/ops/storage/database-retention"

    def request(path, body=None):
        client = connection(target.hostname, target.port, timeout=20)
        try:
            headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
            client.request("GET" if body is None else "POST", route + path,
                           body=None if body is None else json.dumps(body), headers=headers)
            response = client.getresponse()
            if response.status != 200:
                raise APIError(response.status)
            return json.load(response)["data"]
        finally:
            client.close()

    preview = request("/previews", {"expected_revision": revision, "expiry_only": True})
    if (preview.get("state") != "preview" or preview.get("revision") != revision or
            preview.get("policy_snapshot", {}).get("owner_instance") != instance or
            preview.get("cutoffs", {}).get("expiry_only") is not True):
        raise RuntimeError("expiry_preview_or_target_mismatch")
    if not apply:
        return preview
    preview_id = str(uuid.UUID(preview["id"]))
    request_id = str(uuid.uuid4())
    emit({"request_id": request_id, "preview_id": preview_id})
    try:
        result = request("/runs", {"request_id": request_id, "preview_id": preview_id,
                                   "expected_revision": revision, "confirm": True})
    except APIError as exc:
        if exc.status < 500:
            raise
        result = request("/runs/" + request_id)
    except (OSError, http.client.HTTPException, ValueError, KeyError):
        # 响应丢失时只查原回执，由调用方保留 request_id 核对结果。
        result = request("/runs/" + request_id)
    if (result.get("id") != request_id or result.get("preview_id") != preview_id or
            result.get("state") != "completed" or result.get("revision") != revision or
            result.get("cutoffs", {}).get("expiry_only") is not True):
        raise RuntimeError("expiry_receipt_unconfirmed")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--origin", required=True, help="PowerX 地址，可包含自定义 API 前缀")
    parser.add_argument("--instance", required=True)
    parser.add_argument("--revision", type=int, required=True)
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    def emit(row):
        print(json.dumps(row), flush=True)
    result = expiry_batch(args.origin, os.environ.get("POWERX_ADMIN_TOKEN"), str(args.instance),
                          args.revision, args.apply, emit)
    emit({key: result.get(key) for key in ("id", "state", "counts", "scan")})


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, OSError, http.client.HTTPException, KeyError) as exc:
        sys.exit(str(exc))
