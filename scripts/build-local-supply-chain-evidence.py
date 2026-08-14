#!/usr/bin/env python3
"""Create local Development Preview supply-chain evidence.

This is not a production identity signature. It binds the source tree, installed
Python distributions and generated CycloneDX document into an ephemeral Ed25519
signature so the software gate can run without a hosted CI provider. Production
still requires an independently trusted signer and human approval.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.metadata
import json
from pathlib import Path

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workspace", type=Path, default=Path.cwd())
    args = parser.parse_args()
    workspace = args.workspace.resolve()
    destination = workspace / ".ael/supply-chain"
    destination.mkdir(parents=True, exist_ok=True)
    distributions = sorted(
        (
            {
                "name": distribution.metadata.get("Name", "unknown"),
                "version": distribution.version,
                "license": distribution.metadata.get("License") or "UNKNOWN",
            }
            for distribution in importlib.metadata.distributions()
        ),
        key=lambda item: (item["name"].lower(), item["version"]),
    )
    licenses = destination / "python-licenses.json"
    licenses.write_text(json.dumps(distributions, indent=2, sort_keys=True) + "\n")
    components = [
        {
            "type": "library",
            "name": item["name"],
            "version": item["version"],
            "licenses": [{"license": {"name": item["license"]}}],
        }
        for item in distributions
    ]
    sbom = destination / "ael-sbom.cdx.json"
    sbom.write_text(
        json.dumps(
            {
                "bomFormat": "CycloneDX",
                "specVersion": "1.6",
                "version": 1,
                "metadata": {
                    "component": {
                        "type": "application",
                        "name": "agentic-embedded-lab",
                        "version": "0.2.0.dev0",
                    }
                },
                "components": components,
            },
            indent=2,
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )
    source_files = sorted(
        path
        for root in (workspace / "src", workspace / "scripts", workspace / "containers")
        for path in root.rglob("*")
        if path.is_file() and "__pycache__" not in path.parts
    )
    source_digest = hashlib.sha256()
    for path in source_files:
        source_digest.update(str(path.relative_to(workspace)).encode())
        source_digest.update(path.read_bytes())
    signed_payload = {
        "sbom_sha256": sha256(sbom),
        "licenses_sha256": sha256(licenses),
        "source_tree_sha256": source_digest.hexdigest(),
    }
    signed_bytes = json.dumps(
        signed_payload, separators=(",", ":"), sort_keys=True
    ).encode()
    private_key = Ed25519PrivateKey.generate()
    public_key = private_key.public_key()
    signature = private_key.sign(signed_bytes)
    public_key.verify(signature, signed_bytes)
    receipt = {
        "kind": "LocalDevelopmentSignatureReceipt",
        "algorithm": "ed25519",
        "signed_payload": signed_payload,
        "public_key_base64": base64.b64encode(
            public_key.public_bytes(
                encoding=serialization.Encoding.Raw,
                format=serialization.PublicFormat.Raw,
            )
        ).decode(),
        "signature_base64": base64.b64encode(signature).decode(),
        "cryptographic_signature_verified": True,
        "trusted_identity": False,
        "ci_required": False,
        "limitations": [
            "Ephemeral development signer; not a trusted production identity.",
            "Production approval requires an independently trusted signer.",
        ],
    }
    (destination / "ael-sbom.local-signature.json").write_text(
        json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


if __name__ == "__main__":
    main()
