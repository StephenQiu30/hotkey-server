#!/usr/bin/env sh
set -eu

name='hotkey-plan007-minio-fixture'
image='minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e'
port='19007'

require_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    printf '%s\n' 'Docker is required for the PLAN-007 MinIO fixture.' >&2
    exit 1
  fi
  if ! docker info >/dev/null 2>&1; then
    printf '%s\n' 'A running Docker daemon is required for the PLAN-007 MinIO fixture.' >&2
    exit 1
  fi
}

is_running() {
  [ "$(docker inspect --format '{{.State.Running}}' "$name" 2>/dev/null || true)" = 'true' ]
}

wait_until_ready() {
  attempts=0
  while [ "$attempts" -lt 30 ]; do
    if curl --fail --silent --show-error "http://127.0.0.1:${port}/minio/health/ready" >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 1
  done
  docker logs "$name" >&2 || true
  printf '%s\n' 'PLAN-007 MinIO fixture did not become ready.' >&2
  exit 1
}

up() {
  require_docker
  if is_running; then
    wait_until_ready
    printf '%s\n' 'PLAN-007 MinIO fixture already running.'
    return
  fi
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run --detach --rm --name "$name" \
    --publish "127.0.0.1:${port}:9000" \
    --env MINIO_ROOT_USER=hotkey-plan007 \
    --env MINIO_ROOT_PASSWORD=hotkey-plan007-secret \
    "$image" server /data >/dev/null
  wait_until_ready
  printf '%s\n' 'PLAN-007 MinIO fixture ready at 127.0.0.1:19007.'
}

down() {
  require_docker
  docker rm -f "$name" >/dev/null 2>&1 || true
  printf '%s\n' 'PLAN-007 MinIO fixture stopped.'
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  *)
    printf '%s\n' "usage: $0 up|down" >&2
    exit 2
    ;;
esac
