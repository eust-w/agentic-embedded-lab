# SPDX-License-Identifier: Apache-2.0
FROM ubuntu:24.04 AS build
ARG NS3_VERSION=3.47
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      build-essential bzip2 ca-certificates cmake curl ninja-build python3 \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fL "https://www.nsnam.org/releases/ns-allinone-${NS3_VERSION}.tar.bz2" \
      -o /tmp/ns3.tar.bz2 \
    && tar -xjf /tmp/ns3.tar.bz2 -C /opt \
    && cd "/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" \
    && ./ns3 configure --disable-tests --build-profile=release --enable-modules=core \
    && ./ns3 build
FROM ubuntu:24.04
ARG NS3_VERSION=3.47
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libstdc++6 python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build "/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" "/opt/ns-${NS3_VERSION}"
RUN ln -s "/opt/ns-${NS3_VERSION}/ns3" /usr/local/bin/ns3
WORKDIR /workspace
COPY pyproject.toml README.md LICENSE /workspace/
COPY src /workspace/src
RUN python3 -m pip install --break-system-packages --no-cache-dir .
ENV AEL_NS3_BIN=/usr/local/bin/ns3 AEL_WORKSPACE=/workspace
ENTRYPOINT ["python3", "-m", "ael.backend_workers.ns3"]
