"""Ephemeral OIDC-compatible issuer for local Compose acceptance only."""

from __future__ import annotations

import base64
import os
import time
from typing import Any

import jwt
from cryptography.hazmat.primitives.asymmetric import rsa
from fastapi import FastAPI

ISSUER = os.environ.get("AEL_DEV_OIDC_ISSUER", "http://dev-oidc:8080")
AUDIENCE = os.environ.get("AEL_DEV_OIDC_AUDIENCE", "ael-dev")
KEY_ID = "ael-development-ephemeral"
PRIVATE_KEY = rsa.generate_private_key(public_exponent=65537, key_size=2048)


def _b64uint(value: int) -> str:
    width = max(1, (value.bit_length() + 7) // 8)
    return base64.urlsafe_b64encode(value.to_bytes(width, "big")).decode().rstrip("=")


app = FastAPI(title="AEL development OIDC issuer")


@app.get("/.well-known/openid-configuration")
def discovery() -> dict[str, Any]:
    return {
        "issuer": ISSUER,
        "jwks_uri": f"{ISSUER}/.well-known/jwks.json",
        "token_endpoint": f"{ISSUER}/token",
        "id_token_signing_alg_values_supported": ["RS256"],
    }


@app.get("/.well-known/jwks.json")
def jwks() -> dict[str, Any]:
    numbers = PRIVATE_KEY.public_key().public_numbers()
    return {
        "keys": [
            {
                "kty": "RSA",
                "use": "sig",
                "alg": "RS256",
                "kid": KEY_ID,
                "n": _b64uint(numbers.n),
                "e": _b64uint(numbers.e),
            }
        ]
    }


@app.post("/token")
def token() -> dict[str, Any]:
    now = int(time.time())
    encoded = jwt.encode(
        {
            "iss": ISSUER,
            "aud": AUDIENCE,
            "sub": "ael-local-acceptance",
            "iat": now,
            "exp": now + 300,
        },
        PRIVATE_KEY,
        algorithm="RS256",
        headers={"kid": KEY_ID},
    )
    return {"access_token": encoded, "token_type": "Bearer", "expires_in": 300}


def main() -> None:
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8080)


if __name__ == "__main__":
    main()
