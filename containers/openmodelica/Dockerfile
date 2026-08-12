# SPDX-License-Identifier: Apache-2.0
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
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends openmodelica omsimulator \
    && rm -rf /var/lib/apt/lists/* \
    && omc --version | grep -F "${OPENMODELICA_VERSION}" \
    && OMSimulator --version | grep -F "${OMSIMULATOR_VERSION}"
WORKDIR /workspace
COPY pyproject.toml README.md LICENSE /workspace/
COPY src /workspace/src
RUN python3 -m pip install --break-system-packages --no-cache-dir .
ENV AEL_OPENMODELICA_BIN=/usr/bin/omc AEL_OMSIMULATOR_BIN=/usr/bin/OMSimulator AEL_WORKSPACE=/workspace
CMD ["python3", "-m", "ael.backend_workers.modelica"]
