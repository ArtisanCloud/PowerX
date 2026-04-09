#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[docker-install] step 1/3: clean broken docker/env/data"
"${SCRIPT_DIR}/clean.sh" --yes

echo "[docker-install] step 2/3: bootstrap host dirs and env"
"${SCRIPT_DIR}/bootstrap-host.sh"

echo "[docker-install] step 3/3: pull and start services"
"${SCRIPT_DIR}/up.sh"

echo "[docker-install] done"
