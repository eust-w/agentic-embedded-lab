# SPDX-License-Identifier: Apache-2.0
FROM scratch
COPY .ael/container-bin/aether-server /usr/local/bin/aether-server
COPY .ael/container-bin/aether-worker /usr/local/bin/aether-worker
COPY .ael/container-bin/ael-backend /usr/local/bin/ael-backend
USER 10001:10001
WORKDIR /workspace
EXPOSE 8765
ENTRYPOINT ["/usr/local/bin/aether-server"]
CMD ["--mode", "control-plane", "--listen", ":8765"]
