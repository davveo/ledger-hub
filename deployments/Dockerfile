# 默认走 DaoCloud 镜像，避免直连 Docker Hub 超时。海外可覆盖：
#   docker compose build --build-arg GO_IMAGE=golang:1.20-alpine --build-arg RUNTIME_IMAGE=alpine:3.20
ARG GO_IMAGE=docker.m.daocloud.io/library/golang:1.20-alpine
ARG RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.20

FROM ${GO_IMAGE} AS builder
WORKDIR /src
RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.aliyun.com/alpine#g' /etc/apk/repositories \
 && apk add --no-cache git ca-certificates
ENV CGO_ENABLED=0 GOOS=linux GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/connector ./cmd/connector

FROM ${RUNTIME_IMAGE} AS runtime
RUN sed -i 's#https\?://dl-cdn.alpinelinux.org/alpine#https://mirrors.aliyun.com/alpine#g' /etc/apk/repositories \
 && apk add --no-cache ca-certificates tzdata wget \
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
