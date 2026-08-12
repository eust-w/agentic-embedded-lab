# Backend containers

Each directory is an independently built execution image. The Apache-2.0 AEL
core does not redistribute simulator binaries. Image builds fetch the exact
upstream version and fail if the reported version differs. Their resulting
images retain the simulator's own license; see `THIRD_PARTY_NOTICES.md`.

The images expose the versioned JSON-line adapter process on stdin/stdout. They
do not accept shell strings from clients. Workspace mounts should be read-only,
with a dedicated writable evidence volume when deployed.
