# SPDX-License-Identifier: Apache-2.0
FROM golang:1.24-bookworm AS ael-worker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ael-backend ./cmd/ael-backend
COPY internal/ael ./internal/ael
RUN CGO_ENABLED=0 go build -trimpath -o /out/ael-backend ./cmd/ael-backend

FROM ubuntu:24.04

ARG ZEPHYR_VERSION=4.4.2
ARG ZEPHYR_SDK_VERSION=1.0.1
ARG TARGETARCH

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates ccache cmake curl device-tree-compiler file g++ gcc git gperf \
      make ninja-build python3 python3-dev python3-pip python3-setuptools python3-wheel \
      wget xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && python3 -m pip install --break-system-packages --no-cache-dir west

RUN mkdir -p /opt/zephyrproject \
    && git clone --depth=1 --branch "v${ZEPHYR_VERSION}" \
      https://github.com/zephyrproject-rtos/zephyr.git /opt/zephyrproject/zephyr \
    && west init -l /opt/zephyrproject/zephyr \
    && cd /opt/zephyrproject \
    && west update --narrow -o=--depth=1 cmsis cmsis_6 hal_stm32 mcuboot mbedtls \
    && west zephyr-export \
    && python3 -m pip install --break-system-packages --no-cache-dir \
      -r /opt/zephyrproject/zephyr/scripts/requirements-base.txt
RUN python3 -m pip install --break-system-packages --no-cache-dir \
      -r /opt/zephyrproject/bootloader/mcuboot/scripts/requirements.txt

RUN case "${TARGETARCH}" in \
      amd64) SDK_ARCH=x86_64 ;; \
      arm64) SDK_ARCH=aarch64 ;; \
      *) echo "Unsupported Zephyr SDK architecture: ${TARGETARCH}" >&2; exit 2 ;; \
    esac \
    && SDK_ARCHIVE="zephyr-sdk-${ZEPHYR_SDK_VERSION}_linux-${SDK_ARCH}_minimal.tar.xz" \
    && curl --fail --location --retry 6 --retry-all-errors --retry-delay 5 \
      --connect-timeout 20 --max-time 1200 \
      "https://github.com/zephyrproject-rtos/sdk-ng/releases/download/v${ZEPHYR_SDK_VERSION}/${SDK_ARCHIVE}" \
      -o "/tmp/${SDK_ARCHIVE}" \
    && mkdir -p "/opt/zephyr-sdk-${ZEPHYR_SDK_VERSION}" \
    && tar -xf "/tmp/${SDK_ARCHIVE}" --strip-components=1 \
      -C "/opt/zephyr-sdk-${ZEPHYR_SDK_VERSION}" \
    && mkdir -p "/opt/zephyr-sdk-${ZEPHYR_SDK_VERSION}/gnu" \
    && for TOOLCHAIN in arm-zephyr-eabi riscv64-zephyr-elf; do \
      TOOLCHAIN_ARCHIVE="toolchain_gnu_linux-${SDK_ARCH}_${TOOLCHAIN}.tar.xz"; \
      curl --fail --location --retry 6 --retry-all-errors --retry-delay 5 \
        --connect-timeout 20 --max-time 1200 \
        "https://github.com/zephyrproject-rtos/sdk-ng/releases/download/v${ZEPHYR_SDK_VERSION}/${TOOLCHAIN_ARCHIVE}" \
        -o "/tmp/${TOOLCHAIN_ARCHIVE}"; \
      tar -xf "/tmp/${TOOLCHAIN_ARCHIVE}" \
        -C "/opt/zephyr-sdk-${ZEPHYR_SDK_VERSION}/gnu"; \
      rm "/tmp/${TOOLCHAIN_ARCHIVE}"; \
    done \
    && "/opt/zephyr-sdk-${ZEPHYR_SDK_VERSION}/setup.sh" \
      -t arm-zephyr-eabi -t riscv64-zephyr-elf -h -c \
    && rm "/tmp/${SDK_ARCHIVE}"

WORKDIR /workspace
COPY --from=ael-worker /out/ael-backend /usr/local/bin/ael-backend
RUN grep -F 'VERSION_MAJOR = 4' /opt/zephyrproject/zephyr/VERSION \
    && grep -F 'VERSION_MINOR = 4' /opt/zephyrproject/zephyr/VERSION \
    && grep -F 'PATCHLEVEL = 2' /opt/zephyrproject/zephyr/VERSION

ENV AEL_WORKSPACE=/workspace \
    ZEPHYR_BASE=/opt/zephyrproject/zephyr \
    ZEPHYR_SDK_INSTALL_DIR=/opt/zephyr-sdk-1.0.1 \
    ZEPHYR_TOOLCHAIN_VARIANT=zephyr \
    ZEPHYR_TOOLCHAIN_CAPABILITY_CACHE_DIR=/tmp/zephyr-toolchain-cache

ENTRYPOINT ["/usr/local/bin/ael-backend", "--backend", "zephyr_build", "--workspace", "/workspace"]
