#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
image_tag=${DOCKER_VALIDATION_IMAGE:-leo-runners-validation:${DOCKER_VALIDATION_TAG:-$$}}
container_name="leo-runners-validation-$$"
run_smoke=0
keep_image=0

usage() {
    printf '%s\n' "Usage: $0 [--run-smoke] [--keep-image]"
    printf '%s\n' '  --run-smoke    start the image and verify health/webhook responses'
    printf '%s\n' '  --keep-image   retain the validation image after checks complete'
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --run-smoke) run_smoke=1 ;;
        --keep-image) keep_image=1 ;;
        --help|-h) usage; exit 0 ;;
        *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        printf 'Docker validation cannot continue: missing required command: %s\n' "$1" >&2
        exit 2
    }
}

require_command docker

docker_available=1
if ! docker info >/dev/null 2>&1; then
    docker_available=0
fi
if [ "$docker_available" -eq 0 ]; then
    printf '%s\n' 'Docker validation cannot continue: Docker CLI is installed but the daemon is unavailable.' >&2
    printf '%s\n' 'Start Docker Desktop or the Docker daemon, then rerun this script.' >&2
    exit 2
fi

cleanup() {
    status=$?
    docker rm -f "$container_name" >/dev/null 2>&1 || true
    if [ "$keep_image" -eq 0 ]; then
        docker image rm -f "$image_tag" >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup EXIT INT TERM HUP

printf 'building existing root Dockerfile as %s\n' "$image_tag"
docker build --file "$repo_root/Dockerfile" --tag "$image_tag" "$repo_root"

inspect_format='{{json .Config.User}} {{json .Config.ExposedPorts}} {{json .Config.Entrypoint}}'
metadata=$(docker image inspect --format "$inspect_format" "$image_tag")
user=$(printf '%s' "$metadata" | awk '{print $1}' | tr -d '"')
ports=$(printf '%s' "$metadata" | awk '{print $2}')
entrypoint=$(printf '%s' "$metadata" | awk '{print $3}')

[ "$user" = '65532:65532' ] || {
    printf 'non-root check failed: expected UID/GID 65532:65532, got %s\n' "$user" >&2
    exit 1
}
printf '%s\n' 'non-root UID/GID check passed: 65532:65532'

printf '%s' "$ports" | grep -F '8080/tcp' >/dev/null || {
    printf 'port check failed: expected exposed port 8080/tcp, got %s\n' "$ports" >&2
    exit 1
}
printf '%s\n' 'exposed port check passed: 8080/tcp'

[ "$entrypoint" = '["/usr/local/bin/runner-controller"]' ] || {
    printf 'entrypoint check failed: got %s\n' "$entrypoint" >&2
    exit 1
}
printf '%s\n' 'entrypoint check passed: /usr/local/bin/runner-controller'

printf '%s\n' 'checking image filesystem for forbidden secret files'
docker run --rm --entrypoint /bin/sh "$image_tag" -c '
    set -eu
    forbidden=$(find / -xdev -type f \( \
        -name ".env" -o -name ".env.*" -o -name "id_rsa" -o \
        -name "id_ed25519" -o -name "credentials" -o -name "credentials.json" -o \
        -name "*.pem" -o -name "*.key" -o -name "*jitconfig*" \
    \) -print 2>/dev/null || true)
    if [ -n "$forbidden" ]; then
        printf "forbidden secret-like files found:\\n%s\\n" "$forbidden" >&2
        exit 1
    fi
    test ! -e /github-jit-config
    test ! -e /run/secrets
'
printf '%s\n' 'forbidden secret-file check passed'

if [ "$run_smoke" -eq 1 ]; then
    require_command curl
    require_command openssl
    secret="leo-docker-validation-secret-$$"
    printf '%s\n' 'starting optional container health/webhook smoke checks'
    docker run --detach --name "$container_name" \
        --publish 127.0.0.1::8080 \
        --env LISTEN_ADDR=:8080 \
        --env GITHUB_WEBHOOK_SECRET="$secret" \
        --env STATE_PATH=/var/lib/leo-runners/state.json \
        "$image_tag" >/dev/null
    host_port=$(docker port "$container_name" 8080/tcp | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p')
    [ -n "$host_port" ] || { printf '%s\n' 'smoke setup failed: no mapped host port' >&2; exit 1; }
    base_url="http://127.0.0.1:${host_port}"
    ready=0
    i=0
    while [ "$i" -lt 80 ]; do
        if curl -fsS "$base_url/healthz" >/dev/null 2>&1; then ready=1; break; fi
        i=$((i + 1)); sleep 0.1
    done
    [ "$ready" -eq 1 ] || { printf '%s\n' 'smoke check failed: container did not become healthy' >&2; docker logs "$container_name" >&2 || true; exit 1; }
    status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$base_url/webhooks/github")
    [ "$status" = 401 ] || { printf 'smoke check failed: unsigned webhook returned %s, expected 401\n' "$status" >&2; exit 1; }
    body='not-json'
    signature=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $NF}')
    status=$(printf '%s' "$body" | curl -sS -o /dev/null -w '%{http_code}' -X POST \
        -H 'X-GitHub-Event: workflow_job' \
        -H 'X-Hub-Signature-256: sha256='"$signature" \
        --data-binary @- "$base_url/webhooks/github")
    [ "$status" = 400 ] || { printf 'smoke check failed: signed malformed webhook returned %s, expected 400\n' "$status" >&2; exit 1; }
    printf '%s\n' 'optional container smoke checks passed: health, 401, 400'
fi

printf '%s\n' 'Docker validation passed'
