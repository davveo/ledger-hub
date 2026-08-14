# syntax=docker/dockerfile:1

FROM golang:1.20-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/connector ./cmd/connector

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -H -u 65532 app
WORKDIR /app
COPY --from=builder --chown=app:app /src/configs /app/configs
USER app
ENTRYPOINT ["/usr/local/bin/ledger"]
CMD ["-config", "/app/configs/config.docker.yaml"]

FROM runtime AS api
COPY --from=builder --chown=app:app /out/api /usr/local/bin/ledger
EXPOSE 8080

FROM runtime AS gateway
COPY --from=builder --chown=app:app /out/gateway /usr/local/bin/ledger
EXPOSE 8088

FROM runtime AS worker
COPY --from=builder --chown=app:app /out/worker /usr/local/bin/ledger
EXPOSE 8089

FROM runtime AS connector
COPY --from=builder --chown=app:app /out/connector /usr/local/bin/ledger
EXPOSE 8090
