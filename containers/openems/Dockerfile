# SPDX-License-Identifier: Apache-2.0
FROM ubuntu:24.04 AS build
ARG OPENEMS_VERSION=0.0.36
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      build-essential ca-certificates cmake git libboost-all-dev libeigen3-dev libhdf5-dev \
      libtinyxml-dev libvtk9-dev octave octave-dev python3 qtbase5-dev \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --branch "v${OPENEMS_VERSION}" --depth 1 --recurse-submodules \
      https://github.com/thliebig/openEMS-Project.git /tmp/openEMS-Project \
    && cd /tmp/openEMS-Project \
    && ./update_openEMS.sh /opt/openems --disable-GUI
FROM ubuntu:24.04
ARG OPENEMS_VERSION=0.0.36
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libboost-all-dev libhdf5-dev libtinyxml-dev libvtk9-dev octave python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /opt/openems /opt/openems
ENV PATH=/opt/openems/bin:${PATH} OCTAVE_PATH=/opt/openems/share/openEMS/matlab
RUN openEMS --version | grep -F "${OPENEMS_VERSION}"
WORKDIR /workspace
COPY pyproject.toml README.md LICENSE /workspace/
COPY src /workspace/src
RUN python3 -m pip install --break-system-packages --no-cache-dir .
ENV AEL_OPENEMS_BIN=/opt/openems/bin/openEMS AEL_WORKSPACE=/workspace
ENTRYPOINT ["python3", "-m", "ael.backend_workers.openems"]
