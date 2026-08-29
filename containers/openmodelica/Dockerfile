# SPDX-License-Identifier: Apache-2.0
FROM golang:1.24-bookworm AS ael-worker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ael-backend ./cmd/ael-backend
COPY internal/ael ./internal/ael
RUN CGO_ENABLED=0 go build -trimpath -o /out/ael-backend ./cmd/ael-backend

FROM ubuntu:24.04
ARG OPENMODELICA_VERSION=1.27.0
ARG OMSIMULATOR_VERSION=2.1.3
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg python3 python3-pip \
    && curl -fsSL https://build.openmodelica.org/apt/openmodelica.asc \
       | gpg --dearmor -o /usr/share/keyrings/openmodelica.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/openmodelica.gpg] https://build.openmodelica.org/apt noble stable" \
       > /etc/apt/sources.list.d/openmodelica.list \
    && apt-get update \
    # The `openmodelica` metapackage pulls the desktop IDE, notebook and large
    # geospatial/GUI stacks. AEL's headless backend requires only the compiler
    # and FMI coordinator, keeping both CI and local acceptance bounded.
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends omc omsimulator \
    && rm -rf /var/lib/apt/lists/* \
    && omc --version | grep -F "${OPENMODELICA_VERSION}" \
    && OMSimulator --version | grep -F "${OMSIMULATOR_VERSION}"
WORKDIR /workspace
COPY --from=ael-worker /out/ael-backend /usr/local/bin/ael-backend
ENV AEL_OPENMODELICA_BIN=/usr/bin/omc AEL_OMSIMULATOR_BIN=/usr/bin/OMSimulator AEL_WORKSPACE=/workspace
CMD ["/usr/local/bin/ael-backend", "--backend", "openmodelica", "--workspace", "/workspace"]
