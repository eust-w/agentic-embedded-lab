# SPDX-License-Identifier: Apache-2.0
FROM golang:1.24-bookworm AS ael-worker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ael-backend ./cmd/ael-backend
COPY internal/ael ./internal/ael
RUN CGO_ENABLED=0 go build -trimpath -o /out/ael-backend ./cmd/ael-backend

FROM ubuntu:24.04 AS build
ARG NGSPICE_VERSION=46
ARG AEL_DOWNLOAD_PROXY
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      autoconf automake bison build-essential ca-certificates curl flex libtool \
      libreadline-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
RUN if [ -n "${AEL_DOWNLOAD_PROXY}" ]; then export HTTPS_PROXY="${AEL_DOWNLOAD_PROXY}"; fi \
    && curl -fL "https://github.com/imr/ngspice/archive/refs/tags/ngspice-${NGSPICE_VERSION}.tar.gz" \
      -o /tmp/ngspice.tar.gz \
    && tar -xzf /tmp/ngspice.tar.gz -C /tmp \
    && cd "/tmp/ngspice-ngspice-${NGSPICE_VERSION}" \
    && ./autogen.sh \
    && ./configure --prefix=/opt/ngspice --enable-xspice --disable-debug \
    && make -j"$(nproc)" \
    && make install \
    && make distclean \
    && ./configure --prefix=/opt/ngspice-shared --with-ngshared --enable-xspice --disable-debug \
    && make -j"$(nproc)" \
    && make install \
    && mkdir -p /opt/ngspice/lib \
    && cp -a /opt/ngspice-shared/lib/. /opt/ngspice/lib/
FROM ubuntu:24.04
ARG NGSPICE_VERSION=46
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libgomp1 libreadline8t64 python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /opt/ngspice /opt/ngspice
RUN ln -s /opt/ngspice/bin/ngspice /usr/local/bin/ngspice \
    && ngspice --version | grep -F "ngspice-${NGSPICE_VERSION}"
WORKDIR /workspace
COPY --from=ael-worker /out/ael-backend /usr/local/bin/ael-backend
ENV AEL_NGSPICE_BIN=/usr/local/bin/ngspice AEL_WORKSPACE=/workspace \
    LD_LIBRARY_PATH=/opt/ngspice/lib
ENTRYPOINT ["/usr/local/bin/ael-backend", "--backend", "ngspice", "--workspace", "/workspace"]
