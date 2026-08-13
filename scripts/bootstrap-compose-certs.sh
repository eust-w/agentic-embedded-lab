#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cert_dir="${workspace}/.ael/dev-certs"
mkdir -p "${cert_dir}"
chmod 700 "${cert_dir}"

openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -subj "/CN=AEL Development CA" \
  -keyout "${cert_dir}/ca.key" -out "${cert_dir}/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj "/CN=envoy" \
  -keyout "${cert_dir}/server.key" -out "${cert_dir}/server.csr" >/dev/null 2>&1
printf '%s\n' 'subjectAltName=DNS:envoy,DNS:localhost,IP:127.0.0.1' >"${cert_dir}/server.ext"
openssl x509 -req -days 2 -in "${cert_dir}/server.csr" \
  -CA "${cert_dir}/ca.crt" -CAkey "${cert_dir}/ca.key" -CAcreateserial \
  -extfile "${cert_dir}/server.ext" -out "${cert_dir}/server.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj "/CN=compose-worker" \
  -keyout "${cert_dir}/worker.key" -out "${cert_dir}/worker.csr" >/dev/null 2>&1
openssl x509 -req -days 2 -in "${cert_dir}/worker.csr" \
  -CA "${cert_dir}/ca.crt" -CAkey "${cert_dir}/ca.key" -CAcreateserial \
  -out "${cert_dir}/worker.crt" >/dev/null 2>&1
chmod 600 "${cert_dir}"/*.key

fingerprint="$(openssl x509 -in "${cert_dir}/worker.crt" -outform der | shasum -a 256 | awk '{print $1}')"
printf 'AEL_WORKER_FINGERPRINTS=%s\n' "${fingerprint}" >"${cert_dir}/compose.env"
printf '%s\n' "${fingerprint}"
