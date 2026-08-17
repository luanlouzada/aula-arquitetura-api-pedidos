#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "uso: $0 exporter-1|exporter-2|exporter-3" >&2
    exit 2
fi

target=$1
case "$target" in
    exporter-1|exporter-2|exporter-3) ;;
    *)
        echo "instância inválida: $target" >&2
        exit 2
        ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_dir=$(dirname "$script_dir")
cd "$project_dir"

echo "1/3 retirando $target do upstream do NGINX"
docker compose exec -T -e DRAIN_TARGET="$target" nginx sh -eu -c '
    config=/etc/nginx/conf.d/default.conf
    backup=/tmp/default.conf.before-drain
    candidate=/tmp/default.conf.candidate
    cp "$config" "$backup"
    sed "s/server ${DRAIN_TARGET}:8080 max_fails=1 fail_timeout=5s;/server ${DRAIN_TARGET}:8080 max_fails=1 fail_timeout=5s down;/" "$config" > "$candidate"
    cp "$candidate" "$config"
    if ! nginx -t; then
        cp "$backup" "$config"
        exit 1
    fi
    nginx -s reload
'

echo "2/3 NGINX recarregado"
echo "3/3 enviando SIGTERM para $target"
docker compose stop -t 15 "$target"
