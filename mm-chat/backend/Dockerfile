FROM golang:1.25@sha256:d7912cedddfa15b2900a8dfb7187df0af5ec2cb424a371139b5b352fd3e6b740 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-api ./cmd/api \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-admin ./cmd/admin \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-memory-worker ./cmd/memory-worker \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-memory-hindsight-fixture ./cmd/memory-hindsight-fixture \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-memory-regression-capture ./cmd/memory-regression-capture \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-mcp-runner ./cmd/mcp-runner \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-mcp-validate ./cmd/mcp-validate

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS runtime-base
RUN apk add --no-cache ca-certificates curl tzdata \
    && addgroup -S mmchat \
    && adduser -S -G mmchat -H -h /nonexistent -s /sbin/nologin mmchat

FROM runtime-base AS hindsight-fixture
COPY --from=build /out/mm-chat-memory-hindsight-fixture /usr/local/bin/mm-chat-memory-hindsight-fixture
USER mmchat:mmchat
ENTRYPOINT ["/usr/local/bin/mm-chat-memory-hindsight-fixture"]

FROM runtime-base AS memory-regression-capture
COPY --from=build /out/mm-chat-migrate /usr/local/bin/mm-chat-migrate
COPY --from=build /out/mm-chat-memory-regression-capture /usr/local/bin/mm-chat-memory-regression-capture
USER mmchat:mmchat

FROM runtime-base AS runtime
RUN apk add --no-cache bash file git jq nodejs npm py3-pip python3 ripgrep unzip zip \
    && mkdir -p /var/lib/mm-chat/agent-skills /workspace \
    && chown -R mmchat:mmchat /var/lib/mm-chat /workspace
COPY --from=build /out/mm-chat-api /usr/local/bin/mm-chat-api
COPY --from=build /out/mm-chat-migrate /usr/local/bin/mm-chat-migrate
COPY --from=build /out/mm-chat-admin /usr/local/bin/mm-chat-admin
COPY --from=build /out/mm-chat-memory-worker /usr/local/bin/mm-chat-memory-worker
COPY --from=build /out/mm-chat-mcp-validate /usr/local/bin/mm-chat-mcp-validate

USER mmchat:mmchat
EXPOSE 8080
CMD ["/usr/local/bin/mm-chat-api"]

FROM mcr.microsoft.com/playwright/mcp:v0.0.79@sha256:18c0a9c934004fe9580cc79f1e8e6e6cde7c667348b215335e8a23fd3e509804 AS mcp-runner
USER root
RUN test -x /ms-playwright/chromium-1237/chrome-linux64/chrome \
    && apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /opt/google/chrome \
    && ln -s /ms-playwright/chromium-1237/chrome-linux64/chrome /opt/google/chrome/chrome
WORKDIR /opt/mcp-runner
RUN test -x /usr/local/bin/npx
COPY mcp-runner-runtime/package.json mcp-runner-runtime/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts \
    && npm cache clean --force
COPY --from=build /out/mm-chat-mcp-runner /usr/local/bin/mm-chat-mcp-runner
COPY --from=build /out/mm-chat-mcp-validate /usr/local/bin/mm-chat-mcp-validate
USER node:node
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/mm-chat-mcp-runner"]
