FROM python:3.14.6-slim-bookworm@sha256:86f975aca15cf04a40b399eebede9aea7c82eae084d1f1a0a6ef6bcaae871a30 AS builder

ARG UV_VERSION=0.10.11
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_PROJECT_ENVIRONMENT=/app/.venv

RUN pip install --no-cache-dir "uv==${UV_VERSION}"
WORKDIR /build
COPY pyproject.toml uv.lock README.md ./
COPY src ./src
RUN uv sync --frozen --no-dev --no-editable

FROM python:3.14.6-slim-bookworm@sha256:86f975aca15cf04a40b399eebede9aea7c82eae084d1f1a0a6ef6bcaae871a30 AS runtime
ENV PATH="/app/.venv/bin:${PATH}" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

RUN groupadd --system --gid 10001 rag \
    && useradd --system --uid 10001 --gid rag --home-dir /nonexistent rag \
    && useradd --system --uid 10002 --gid rag --home-dir /nonexistent parser
WORKDIR /app
COPY --from=builder --chown=rag:rag /app/.venv /app/.venv
USER 10001:10001
EXPOSE 8081
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8081/health', timeout=2).read()"]
ENTRYPOINT ["rag-worker"]
