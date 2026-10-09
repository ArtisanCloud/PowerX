#!/bin/sh
set -eu
umask 077
export POWERX_CONFIG=${POWERX_CONFIG:-/etc/powerx/config.yaml}
export POWERX_MODE=docker
case ${1:-serve} in
    init) exec python3 /app/container/init.py ;;
    bootstrap)
        echo 'Automatic bootstrap has been removed. Complete /setup to create your administrator and initialize the database.' >&2
        exit 1 ;;
    credentials) echo 'Administrator credentials are chosen by you in /setup. No default password is generated.' ;;
    setup-values)
        exec python3 -c 'import yaml,json; c=yaml.safe_load(open("/etc/powerx/config.yaml")); print(json.dumps({"database":c["database"],"cache":c["cache"],"storage":c["storage"]["local"],"deployment":c["deployment"]},ensure_ascii=False))' ;;
    migrate) exec /app/database migrate ;;
    status) exec /app/database status ;;
    serve)
        [ -s "$POWERX_CONFIG" ] || { echo 'Run init first; no database reset is performed at startup.' >&2; exit 1; }
        exec /app/powerx-app ;;
    *) exec "$@" ;;
esac
