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
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libstdc++6 python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build "/opt/ns-allinone-${NS3_VERSION}/ns-${NS3_VERSION}" "/opt/ns-${NS3_VERSION}"
COPY --from=build /opt/ael-bin /opt/ael-bin
RUN printf '%s\n' "${NS3_VERSION}" > "/opt/ns-${NS3_VERSION}/.ael-version" \
    && ln -s "/opt/ns-${NS3_VERSION}/ns3" /usr/local/bin/ns3
WORKDIR /workspace
COPY pyproject.toml README.md LICENSE /workspace/
COPY src /workspace/src
RUN python3 -m pip install --break-system-packages --no-cache-dir .
ENV AEL_NS3_BIN=/usr/local/bin/ns3 AEL_WORKSPACE=/workspace \
    AEL_NS3_PRECOMPILED=/opt/ael-bin/ael-network \
    AEL_NS3_MODEL_SHA256=/opt/ael-bin/ael-network.sha256 \
    LD_LIBRARY_PATH=/opt/ns-${NS3_VERSION}/build/lib
RUN /opt/ael-bin/ael-network --stopUs=1000 | grep -F "AEL_METRIC failure=0"
ENTRYPOINT ["python3", "-m", "ael.backend_workers.ns3"]
