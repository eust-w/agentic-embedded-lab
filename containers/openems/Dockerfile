# SPDX-License-Identifier: Apache-2.0
FROM golang:1.24-bookworm AS ael-worker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ael-backend ./cmd/ael-backend
COPY internal/ael ./internal/ael
RUN CGO_ENABLED=0 go build -trimpath -o /out/ael-backend ./cmd/ael-backend

FROM ubuntu:24.04 AS build
ARG OPENEMS_VERSION=0.0.36
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
        build-essential ca-certificates cmake git libboost-all-dev libcgal-dev libeigen3-dev libhdf5-dev \
        libtinyxml-dev libvtk9-dev octave octave-dev python3 qtbase5-dev
RUN if [ -n "${AEL_DOWNLOAD_PROXY}" ]; then export HTTPS_PROXY="${AEL_DOWNLOAD_PROXY}"; fi \
    && git clone --branch "v${OPENEMS_VERSION}" --depth 1 --recurse-submodules \
      https://github.com/thliebig/openEMS-Project.git /tmp/openEMS-Project \
    && cd /tmp/openEMS-Project \
    && { ./update_openEMS.sh /opt/openems --disable-GUI \
      || { cat build_*.log; exit 1; }; } \
    && mkdir -p /opt/openems-runtime-libs \
    && { find /opt/openems -type f \( -perm -u+x -o -name '*.so*' -o -name '*.oct' -o -name '*.mex*' \) \
         -exec ldd {} \; 2>/dev/null || true; } \
       | awk '/=> \/.* \(0x/ {print $3} /^\// {print $1}' \
       | sort -u \
       | while read -r library; do cp -Ln "${library}" /opt/openems-runtime-libs/; done
FROM ubuntu:24.04
ARG OPENEMS_VERSION=0.0.36
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
      install -y --no-install-recommends libtinyxml2.6.2v5 octave python3 python3-pip
COPY --from=build /opt/openems /opt/openems
COPY --from=build /opt/openems-runtime-libs /opt/openems-runtime-libs
RUN cp -a /opt/openems-runtime-libs/. /usr/local/lib/ && ldconfig
ENV PATH=/opt/openems/bin:${PATH} \
    LD_LIBRARY_PATH=/opt/openems/lib:/opt/openems-runtime-libs \
    OCTAVE_PATH=/opt/openems/share/openEMS/matlab:/opt/openems/share/CSXCAD/matlab
RUN openEMS --version | grep -F "${OPENEMS_VERSION}"
RUN octave-cli --no-gui --quiet --eval \
      "[status,out]=system('openEMS --version 2>&1'); disp(out); \
       assert(isempty(strfind(out,'error while loading shared libraries'))); \
       assert(!isempty(strfind(out,'version v${OPENEMS_VERSION}')));"
WORKDIR /workspace
COPY --from=ael-worker /out/ael-backend /usr/local/bin/ael-backend
ENV AEL_OPENEMS_BIN=/opt/openems/bin/openEMS AEL_WORKSPACE=/workspace
ENTRYPOINT ["/usr/local/bin/ael-backend", "--backend", "openems", "--workspace", "/workspace"]
