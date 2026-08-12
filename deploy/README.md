# Deployment boundary

`compose.yaml` is a local recovery/semantics environment. It deliberately uses
development credentials and is not production configuration.

The Helm chart supplies rolling deployment, probes, non-root/seccomp settings,
PostgreSQL/S3 wiring, and mandatory OIDC configuration. A production ingress
must terminate worker mTLS and overwrite `X-Client-Cert-SHA256` with the SHA-256
fingerprint of the verified peer certificate; `envoy.yaml` shows that boundary.
The control-plane container must not be exposed directly where worker routes are
reachable.

Rollback uses `helm rollback <release> <revision>`. Database migrations are
additive in this development version; a production gate still requires backup,
restore, lease recovery, and migration rollback evidence.
