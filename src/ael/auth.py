from __future__ import annotations

import hashlib
import os
from dataclasses import dataclass
from typing import Any


class AuthenticationError(PermissionError):
    pass


@dataclass(frozen=True)
class OidcConfig:
    issuer: str
    audience: str
    jwks_url: str | None = None


class OidcVerifier:
    def __init__(self, config: OidcConfig) -> None:
        try:
            import jwt
        except ImportError as exception:
            raise RuntimeError("install AEL with the server extra") from exception
        self.jwt = jwt
        self.config = config
        url = config.jwks_url or f"{config.issuer.rstrip('/')}/.well-known/jwks.json"
        self.jwks = jwt.PyJWKClient(url)

    def verify(self, token: str) -> dict[str, Any]:
        try:
            signing_key = self.jwks.get_signing_key_from_jwt(token)
            return self.jwt.decode(
                token,
                signing_key.key,
                algorithms=["RS256", "ES256"],
                audience=self.config.audience,
                issuer=self.config.issuer,
                options={"require": ["exp", "iat", "sub"]},
            )
        except Exception as exception:
            raise AuthenticationError("invalid OIDC access token") from exception


def oidc_config_from_environment() -> OidcConfig | None:
    issuer = os.environ.get("AEL_OIDC_ISSUER")
    audience = os.environ.get("AEL_OIDC_AUDIENCE")
    if not issuer and not audience:
        return None
    if not issuer or not audience:
        raise ValueError("AEL_OIDC_ISSUER and AEL_OIDC_AUDIENCE must be configured together")
    return OidcConfig(issuer, audience, os.environ.get("AEL_OIDC_JWKS_URL"))


def certificate_fingerprint(der_certificate: bytes) -> str:
    return hashlib.sha256(der_certificate).hexdigest()


def verify_worker_fingerprint(presented: str, declared: str) -> None:
    normalized = presented.strip().lower().replace(":", "")
    if len(normalized) != 64 or normalized != declared.lower():
        raise AuthenticationError("worker certificate fingerprint mismatch")
    allowed = {
        value.strip().lower().replace(":", "")
        for value in os.environ.get("AEL_WORKER_FINGERPRINTS", "").split(",")
        if value.strip()
    }
    if not allowed and os.environ.get("AEL_ALLOW_INSECURE_WORKER_TESTS") != "1":
        raise AuthenticationError("no worker certificate allowlist is configured")
    if allowed and normalized not in allowed:
        raise AuthenticationError("worker certificate is not allow-listed")
