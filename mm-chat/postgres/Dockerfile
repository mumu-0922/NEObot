ARG POSTGRES_BASE_IMAGE="docker.io/library/postgres:17.10-bookworm@sha256:4f736ae292687621d4dbe0d499ffd024a36bd2ee7d8ca6f2ccd4c800f047b394"

FROM ${POSTGRES_BASE_IMAGE} AS extension-build

ARG PG_TEXTSEARCH_VERSION="1.3.1"
ARG PG_TEXTSEARCH_REVISION="578ff529894992fb9e67cae4c69424e65c84868e"
ARG PG_TEXTSEARCH_SHA256="8632f91231251dc3e19395ef6a0d4d158d5f5920ba420691471771418e2a7cc7"
ARG PGVECTOR_VERSION="0.8.5"
ARG PGVECTOR_REVISION="159b79aaad5983fb7459c1e3df2897fbb2d11788"
ARG PGVECTOR_SHA256="9a483fad70ae2e0a50b3dccb6c4b4931d9a07375a1d5815e82b57870448a7d52"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      curl \
      postgresql-server-dev-17 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

RUN set -eux; \
    curl -fsSL --retry 3 \
      -o pg_textsearch.tar.gz \
      "https://github.com/timescale/pg_textsearch/archive/${PG_TEXTSEARCH_REVISION}.tar.gz"; \
    echo "${PG_TEXTSEARCH_SHA256}  pg_textsearch.tar.gz" | sha256sum -c -; \
    mkdir pg_textsearch; \
    tar -xzf pg_textsearch.tar.gz --strip-components=1 -C pg_textsearch; \
    make -C pg_textsearch -j"$(nproc)" PG_CONFIG=/usr/lib/postgresql/17/bin/pg_config; \
    make -C pg_textsearch PG_CONFIG=/usr/lib/postgresql/17/bin/pg_config \
      DESTDIR=/opt/extensions install; \
    install -D -m 0644 pg_textsearch/LICENSE \
      "/opt/extensions/usr/share/doc/pg_textsearch-${PG_TEXTSEARCH_VERSION}/LICENSE"; \
    curl -fsSL --retry 3 \
      -o pgvector.tar.gz \
      "https://github.com/pgvector/pgvector/archive/${PGVECTOR_REVISION}.tar.gz"; \
    echo "${PGVECTOR_SHA256}  pgvector.tar.gz" | sha256sum -c -; \
    mkdir pgvector; \
    tar -xzf pgvector.tar.gz --strip-components=1 -C pgvector; \
    make -C pgvector -j"$(nproc)" OPTFLAGS="" \
      PG_CONFIG=/usr/lib/postgresql/17/bin/pg_config; \
    make -C pgvector OPTFLAGS="" PG_CONFIG=/usr/lib/postgresql/17/bin/pg_config \
      DESTDIR=/opt/extensions install; \
    install -D -m 0644 pgvector/LICENSE \
      "/opt/extensions/usr/share/doc/pgvector-${PGVECTOR_VERSION}/LICENSE"

FROM ${POSTGRES_BASE_IMAGE}

ARG PG_TEXTSEARCH_VERSION="1.3.1"
ARG PG_TEXTSEARCH_REVISION="578ff529894992fb9e67cae4c69424e65c84868e"
ARG PGVECTOR_VERSION="0.8.5"
ARG PGVECTOR_REVISION="159b79aaad5983fb7459c1e3df2897fbb2d11788"

LABEL org.opencontainers.image.title="mm-chat PostgreSQL retrieval runtime" \
      org.opencontainers.image.description="PostgreSQL 17 with pg_textsearch and pgvector" \
      org.opencontainers.image.licenses="PostgreSQL" \
      com.mm-chat.postgresql.version="17.10" \
      com.mm-chat.pg-textsearch.version="${PG_TEXTSEARCH_VERSION}" \
      com.mm-chat.pg-textsearch.revision="${PG_TEXTSEARCH_REVISION}" \
      com.mm-chat.pgvector.version="${PGVECTOR_VERSION}" \
      com.mm-chat.pgvector.revision="${PGVECTOR_REVISION}"

COPY --from=extension-build /opt/extensions/ /
COPY docker-entrypoint-guard.sh /usr/local/bin/mm-chat-postgres-entrypoint
COPY initdb/001-retrieval-extensions.sql /docker-entrypoint-initdb.d/001-retrieval-extensions.sql

RUN chmod 0755 /usr/local/bin/mm-chat-postgres-entrypoint

ENTRYPOINT ["/usr/local/bin/mm-chat-postgres-entrypoint"]
CMD ["postgres", "-c", "shared_preload_libraries=pg_textsearch"]
