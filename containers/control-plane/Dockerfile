# SPDX-License-Identifier: Apache-2.0
FROM python:3.12-slim-bookworm
RUN useradd --create-home --uid 10001 ael
WORKDIR /app
COPY pyproject.toml README.md LICENSE /app/
COPY src /app/src
RUN pip install --no-cache-dir '.[server,mcp]' \
    && mkdir -p /workspace \
    && chown -R ael:ael /workspace
USER 10001
ENV AEL_WORKSPACE=/workspace
EXPOSE 8765
ENTRYPOINT ["uvicorn", "ael.api:app", "--host", "0.0.0.0", "--port", "8765"]
