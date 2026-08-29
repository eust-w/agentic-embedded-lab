# SPDX-License-Identifier: Apache-2.0
FROM golang:1.24-bookworm AS ael-worker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ael-backend ./cmd/ael-backend
COPY internal/ael ./internal/ael
RUN CGO_ENABLED=0 go build -trimpath -o /out/ael-backend ./cmd/ael-backend

FROM ubuntu:24.04 AS build
ARG NS3_VERSION=3.47
ARG AEL_DOWNLOAD_PROXY
ARG AEL_APT_MIRROR=http://ports.ubuntu.com/ubuntu-ports
RUN sed -i "s|http://ports.ubuntu.com/ubuntu-ports|${AEL_APT_MIRROR}|g" \
      /etc/apt/sources.list.d/ubuntu.sources
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get -o Acquire::Retries=8 update \
    && DEBIAN_FRONTEND=noninteractive apt-get \
      -o Acquire::Retries=8 \
      -o Acquire::http::Timeout=60 \
      -o Acquire::https::Timeout=60 \
      -o Binary::apt::APT::Keep-Downloaded-Packages=true \
      install -y --no-install-recommends \
        build-essential bzip2 ca-certificates cmake curl ninja-build python3
RUN if [ -n "${AEL_DOWNLOAD_PROXY}" ]; then export HTTPS_PROXY="${AEL_DOWNLOAD_PROXY}"; fi \
    && curl --fail --location --retry 6 --retry-all-errors --retry-delay 5 \
      --connect-timeout 20 --max-time 600 \
      "https://www.nsnam.org/releases/ns-allinone-${NS3_VERSION}.tar.bz2" \
      -o /tmp/ns3.tar.bz2 \
    && tar -xjf /tmp/ns3.tar.bz2 -C /opt \
    && cd "/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" \
    && ./ns3 configure --disable-tests --build-profile=release \
         --enable-modules=core,network,mobility,spectrum,lr-wpan,energy,wifi \
    && ./ns3 build
COPY benchmarks/models/ns3/ael-network.cc /tmp/ael-network.cc
RUN root="/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" \
    && cp /tmp/ael-network.cc "${root}/scratch/ael-network.cc" \
    && cd "${root}" \
    && ./ns3 build \
    && binary="$(find build -type f -perm -111 -name '*ael-network*' | head -n 1)" \
    && test -n "${binary}" \
    && install -D "${binary}" /opt/ael-bin/ael-network \
    && sha256sum /tmp/ael-network.cc | cut -d' ' -f1 > /opt/ael-bin/ael-network.sha256
FROM ubuntu:24.04
ARG NS3_VERSION=3.47
ARG AEL_APT_MIRROR=http://ports.ubuntu.com/ubuntu-ports
RUN sed -i "s|http://ports.ubuntu.com/ubuntu-ports|${AEL_APT_MIRROR}|g" \
      /etc/apt/sources.list.d/ubuntu.sources
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get -o Acquire::Retries=8 update \
    && DEBIAN_FRONTEND=noninteractive apt-get \
      -o Acquire::Retries=8 \
      -o Acquire::http::Timeout=60 \
      -o Acquire::https::Timeout=60 \
      -o Binary::apt::APT::Keep-Downloaded-Packages=true \
      install -y --no-install-recommends libstdc++6 python3 python3-pip
COPY --from=build "/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" "/opt/ns-${NS3_VERSION}"
COPY --from=build /opt/ael-bin /opt/ael-bin
RUN printf '%s\n' "${NS3_VERSION}" > "/opt/ns-${NS3_VERSION}/.ael-version" \
    && ln -s "/opt/ns-${NS3_VERSION}/ns3" /usr/local/bin/ns3
WORKDIR /workspace
COPY --from=ael-worker /out/ael-backend /usr/local/bin/ael-backend
ENV AEL_NS3_BIN=/usr/local/bin/ns3 AEL_WORKSPACE=/workspace \
    AEL_NS3_PRECOMPILED=/opt/ael-bin/ael-network \
    AEL_NS3_MODEL_SHA256=/opt/ael-bin/ael-network.sha256 \
    LD_LIBRARY_PATH=/opt/ns-${NS3_VERSION}/build/lib
ENTRYPOINT ["/usr/local/bin/ael-backend", "--backend", "ns3", "--workspace", "/workspace"]
