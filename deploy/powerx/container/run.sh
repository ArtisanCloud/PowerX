#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
[ -f .env ] || { echo 'Copy .env.example to .env and select a published image tag first.' >&2; exit 1; }
compose() { docker compose "$@"; }
case ${1:-help} in
    init) compose run --rm --no-deps init init ;;
    start)
        compose run --rm --no-deps init init
        compose up -d --wait --wait-timeout 180 postgres redis
        compose run --rm --no-deps init bootstrap
        compose --profile standalone up -d --wait --wait-timeout 300 backend web-admin gateway
        ;;
    credentials) compose run --rm --no-deps init credentials ;;
    status) compose --profile standalone ps ;;
    logs) compose logs --tail 100 backend web-admin ;;
    migrate) compose run --rm --no-deps init migrate ;;
    stop) compose --profile standalone stop ;;
    *) echo 'Usage: sh run.sh init|start|credentials|status|logs|migrate|stop' ;;
esac
