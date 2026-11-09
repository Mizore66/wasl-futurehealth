# syntax=docker/dockerfile:1
# NATS with the Wasl cluster config baked in. A host bind mount of nats.conf is unreadable when the container VM
# (for example a Podman machine) does not share the checkout directory, so the config is copied into the image.
# It holds no key: the JetStream key is read from $WASL_JS_KEY in the container environment.
# entrypoint.sh refuses to start on retained stores with a wrong or missing key (nats-server would otherwise start,
# fail to decrypt them and let the cluster reset the streams).
# Build the pinned server with the patched Go toolchain; the upstream 2.12.1 binary carries known stdlib findings.
FROM golang:1.27.3-alpine AS build
WORKDIR /src
RUN go mod init wasl-nats-build \
    && go get github.com/nats-io/nats-server/v2@v2.12.6 \
    && go get github.com/klauspost/compress@v1.18.7 golang.org/x/crypto@v0.55.0 \
    && CGO_ENABLED=0 go build -o /go/bin/nats-server github.com/nats-io/nats-server/v2

FROM nats:2.12.6-alpine AS upstream

FROM alpine:3.23
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S wasl \
    && adduser -S -D -H -u 10001 -G wasl wasl \
    && mkdir -p /data \
    && chown -R 10001:10001 /data
COPY --from=build /go/bin/nats-server /usr/local/bin/nats-server
COPY --from=upstream /usr/local/bin/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY nats.conf /etc/nats/wasl.conf
COPY nats-production.conf /etc/nats/wasl-production.conf
COPY --chmod=0755 entrypoint.sh /usr/local/bin/wasl-nats-entrypoint.sh
RUN chown -R 10001:10001 /data /etc/nats
USER 10001:10001
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]
CMD ["nats-server", "-c", "/etc/nats/wasl.conf"]
