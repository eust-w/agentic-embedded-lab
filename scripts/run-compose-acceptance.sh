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
python_bin="${AEL_PYTHON:-python3}"
if [[ -x "${workspace}/.venv/bin/python" ]]; then
  python_bin="${workspace}/.venv/bin/python"
fi
"${python_bin}" "${workspace}/scripts/compose_acceptance.py" \
  --workspace "${workspace}" \
  --output "${workspace}/.ael/compose-acceptance/report.json"
