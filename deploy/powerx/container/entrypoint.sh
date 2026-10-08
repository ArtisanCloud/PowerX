#!/bin/sh
set -eu
umask 077
export POWERX_CONFIG=${POWERX_CONFIG:-/etc/powerx/config.yaml}
export POWERX_MODE=docker
case ${1:-serve} in
    init) exec python3 /app/container/init.py ;;
    bootstrap)
        if [ -f /etc/powerx/initialized ]; then
            echo 'Existing installation preserved; initialization was not repeated.'
            exit 0
        fi
        [ -s /etc/powerx/setup.wizard.config.json ] || { echo 'Run init first; refusing default administrator credentials.' >&2; exit 1; }
        /app/database migrate
        /app/database seed
        printf 'initialized\n' > /etc/powerx/initialized
        rm /etc/powerx/setup.wizard.config.json
        echo 'Bootstrap completed. Read your initial credentials through the credentials command.'
        ;;
    credentials)
        [ -f /etc/powerx/initialized ] || { echo 'Bootstrap not completed.' >&2; exit 1; }
        exec cat /etc/powerx/initial-admin.json ;;
    migrate) exec /app/database migrate ;;
    status) exec /app/database status ;;
    serve)
        [ -f /etc/powerx/initialized ] || { echo 'Run init and bootstrap first; no database reset is performed at startup.' >&2; exit 1; }
        exec /app/powerx-app ;;
    *) exec "$@" ;;
esac
