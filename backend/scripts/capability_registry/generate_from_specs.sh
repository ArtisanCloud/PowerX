#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   backend/scripts/capability_registry/generate_from_specs.sh \
#     --openapi backend/api/openapi/swagger.json \
#     --proto backend/api/grpc/contracts \
#     --out backend/config/platform_capabilities/generated.auto.yaml

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"
mkdir -p .gocache .gomodcache
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.gomodcache}"

OPENAPI_ARGS=()
PROTO_ARGS=()
GIN_ARGS=(--gin-src internal/transport/http)
OUT="backend/config/platform_capabilities/generated.auto.yaml"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --openapi)
      OPENAPI_ARGS+=("--openapi" "$2")
      shift 2
      ;;
    --proto)
      PROTO_ARGS+=("--proto" "$2")
      shift 2
      ;;
    --out)
      OUT="$2"
      shift 2
      ;;
    --gin-src)
      GIN_ARGS+=(--gin-src "$2")
      shift 2
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 1
      ;;
  esac
done

if [[ ${#OPENAPI_ARGS[@]} -eq 0 && ${#PROTO_ARGS[@]} -eq 0 ]]; then
  echo "at least one --openapi or --proto is required" >&2
  exit 1
fi

CMD=(go run ./cmd/capability_gen "${OPENAPI_ARGS[@]}" "${PROTO_ARGS[@]}" "${GIN_ARGS[@]}" --out "$OUT")
if [[ "$DRY_RUN" == "true" ]]; then
  CMD+=(--dry-run)
fi

"${CMD[@]}"
