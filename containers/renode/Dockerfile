# SPDX-License-Identifier: Apache-2.0
FROM ubuntu:24.04
ARG RENODE_VERSION=1.16.1
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates curl libglib2.0-0 libgtk2.0-0 libx11-6 python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/* \
    && curl --fail --location --retry 6 --retry-all-errors --retry-delay 5 \
       --connect-timeout 20 --max-time 600 \
       "https://github.com/renode/renode/releases/download/v${RENODE_VERSION}/renode-${RENODE_VERSION}.linux-portable.tar.gz" \
       -o /tmp/renode.tar.gz \
    && mkdir -p /opt/renode \
    && tar -xzf /tmp/renode.tar.gz --strip-components=1 -C /opt/renode \
    && ln -s /opt/renode/renode /usr/local/bin/renode \
    && rm /tmp/renode.tar.gz \
    && renode --version | grep -F "${RENODE_VERSION}"
WORKDIR /workspace
COPY pyproject.toml README.md LICENSE /workspace/
COPY src /workspace/src
RUN python3 -m pip install --break-system-packages --no-cache-dir .
ENV AEL_RENODE_BIN=/usr/local/bin/renode AEL_WORKSPACE=/workspace
ENTRYPOINT ["python3", "-m", "ael.backend_workers.renode"]
