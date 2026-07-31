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
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-memory-regression-capture ./cmd/memory-regression-capture

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
COPY --from=build /out/mm-chat-api /usr/local/bin/mm-chat-api
COPY --from=build /out/mm-chat-migrate /usr/local/bin/mm-chat-migrate
COPY --from=build /out/mm-chat-admin /usr/local/bin/mm-chat-admin
COPY --from=build /out/mm-chat-memory-worker /usr/local/bin/mm-chat-memory-worker

USER mmchat:mmchat
EXPOSE 8080
CMD ["/usr/local/bin/mm-chat-api"]
