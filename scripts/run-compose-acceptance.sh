#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
active_context="$(docker context show)"
docker_host="$(docker context inspect "${active_context}" --format '{{.Endpoints.docker.Host}}')"
docker_config="${workspace}/.ael/docker-config"
mkdir -p "${docker_config}/cli-plugins"
printf '%s\n' '{"auths":{}}' >"${docker_config}/config.json"
ln -sfn "$(command -v docker-compose)" "${docker_config}/cli-plugins/docker-compose"
export DOCKER_CONFIG="${docker_config}"
export DOCKER_HOST="${docker_host}"
compose=(docker compose --project-name ael-acceptance -f "${workspace}/deploy/compose.yaml")

container_arch="arm64"
if [[ "$(uname -m)" == "x86_64" ]]; then
  container_arch="amd64"
fi
mkdir -p "${workspace}/.ael/container-bin"
for command in aether-server aether-worker ael-backend; do
  CGO_ENABLED=0 GOOS=linux GOARCH="${container_arch}" GOTOOLCHAIN=local \
    GOCACHE="${AEL_GO_CACHE:-/tmp/aether-go-cache}" \
    go build -trimpath -ldflags='-s -w' \
      -o "${workspace}/.ael/container-bin/${command}" "${workspace}/cmd/${command}"
done

# A previous interrupted acceptance run may still hold the old, in-memory TLS
# certificate. Tear it down before rotating the ephemeral development CA.
"${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
fingerprint="$("${workspace}/scripts/bootstrap-compose-certs.sh")"
export AEL_WORKER_FINGERPRINTS="${fingerprint}"

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

"${compose[@]}" up --build --detach --wait
GOTOOLCHAIN=local GOCACHE="${AEL_GO_CACHE:-/tmp/aether-go-cache}" \
  go run "${workspace}/cmd/aether-compose-acceptance" \
  --workspace "${workspace}" \
  --output "${workspace}/.ael/compose-acceptance/report.json"
