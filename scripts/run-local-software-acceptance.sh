#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${workspace}"

if ! docker info >/dev/null 2>&1; then
  echo "A running Docker-compatible Linux VM is required (for example Colima)." >&2
  exit 2
fi

revision="$(git rev-parse HEAD)"
export AEL_WORKSPACE="${workspace}"
export AEL_OCI_RUNTIME=docker
export AEL_ZEPHYR_BUILD_IMAGE=ael-zephyr:local
export AEL_RENODE_IMAGE=ael-renode:local
export AEL_NGSPICE_IMAGE=ael-ngspice:local
export AEL_OPENMODELICA_IMAGE=ael-openmodelica:local
export AEL_OMSIMULATOR_IMAGE=ael-openmodelica:local
export AEL_NS3_IMAGE=ael-ns3:local
export AEL_OPENEMS_IMAGE=ael-openems:local

docker_build_args=()
docker_build_proxy="${AEL_DOCKER_BUILD_PROXY:-}"
if [[ -z "${docker_build_proxy}" ]] && command -v colima >/dev/null 2>&1; then
  docker_build_proxy="$(colima ssh -- printenv HTTPS_PROXY 2>/dev/null || true)"
fi
if [[ -n "${docker_build_proxy}" ]]; then
  docker_build_args+=(
    --build-arg "HTTP_PROXY=${docker_build_proxy}"
    --build-arg "HTTPS_PROXY=${docker_build_proxy}"
  )
fi

for backend in zephyr renode ngspice openmodelica ns3 openems; do
  docker build \
    "${docker_build_args[@]}" \
    --file "containers/${backend}/Dockerfile" \
    --tag "ael-${backend}:local" \
    .
done

docker run --rm \
  --user="$(id -u):$(id -g)" \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs=/tmp:rw,exec,nosuid,nodev,size=2g \
  --mount "type=bind,src=${workspace},dst=/workspace" \
  --workdir=/workspace \
  --env=AEL_ZEPHYR_CACHE_DIR=/workspace/.ael/zephyr-cache \
  --env=HOME=/tmp \
  --entrypoint=bash \
  "${AEL_ZEPHYR_BUILD_IMAGE}" \
  scripts/build-firmware.sh

.venv/bin/ael benchmark run --source-revision "${revision}"
scripts/run-fmi-acceptance.py --om-simulator scripts/omsimulator-container
scripts/check-determinism.py \
  benchmarks/cases/24-antenna-cross-domain/fixed.yaml --warmup --repeats 20
.venv/bin/ael release check --profile simulation

scripts/run-compose-acceptance.sh
.venv/bin/pytest \
  tests/test_worker.py tests/test_server_storage.py tests/test_security.py \
  tests/test_api.py tests/test_release.py --junitxml=.ael/software-tests.xml
scripts/build-local-supply-chain-evidence.py --workspace "${workspace}"
scripts/build-software-acceptance.py \
  --workspace "${workspace}" \
  --compose-report .ael/compose-acceptance/report.json \
  --junit .ael/software-tests.xml \
  --sbom .ael/supply-chain/ael-sbom.cdx.json \
  --signature .ael/supply-chain/ael-sbom.local-signature.json \
  --licenses .ael/supply-chain/python-licenses.json \
  --source-revision "${revision}"
.venv/bin/ael release check --profile software
