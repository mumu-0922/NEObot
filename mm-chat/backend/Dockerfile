# syntax=docker/dockerfile:1.7

FROM golang:1.25@sha256:d7912cedddfa15b2900a8dfb7187df0af5ec2cb424a371139b5b352fd3e6b740 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-api ./cmd/api \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/mm-chat-admin ./cmd/admin

FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
RUN apk add --no-cache ca-certificates curl tzdata \
    && addgroup -S mmchat \
    && adduser -S -G mmchat -H -h /nonexistent -s /sbin/nologin mmchat

COPY --from=build /out/mm-chat-api /usr/local/bin/mm-chat-api
COPY --from=build /out/mm-chat-migrate /usr/local/bin/mm-chat-migrate
COPY --from=build /out/mm-chat-admin /usr/local/bin/mm-chat-admin

USER mmchat:mmchat
EXPOSE 8080
CMD ["/usr/local/bin/mm-chat-api"]
