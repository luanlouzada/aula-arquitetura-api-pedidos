#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo "uso: $0 api-1|api-2|api-3" >&2
    exit 2
fi

target=$1
case "$target" in
    api-1|api-2|api-3) ;;
    *)
        echo "instância inválida: $target (use api-1, api-2 ou api-3)" >&2
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

echo "2/3 NGINX recarregado; novas conexões não escolhem $target"
echo "3/3 enviando SIGTERM e aguardando o shutdown gracioso de $target"
docker compose stop -t 15 "$target"

echo "$target saiu; use 'make lab-reset' para recriar as três instâncias"
