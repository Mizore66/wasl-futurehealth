# syntax=docker/dockerfile:1
# Rebuild NATS with a supported Go toolchain and patched modules: the upstream 2.12.6 image still bundles an older
# Go runtime and x/crypto. The final stage retains the official image layout and receives Alpine security updates.
ARG GO_VERSION=1.27.3
ARG NATS_VERSION=2.12.6

FROM golang:${GO_VERSION}-bookworm AS build
ARG NATS_VERSION
WORKDIR /src
RUN go mod init wasl-nats-build \
 && go get github.com/nats-io/nats-server/v2@v${NATS_VERSION} \
    golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2 \
    golang.org/x/sys@v0.49.0 \
    github.com/klauspost/compress@v1.20.0 \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/nats-server github.com/nats-io/nats-server/v2

# NATS with the Wasl cluster config baked in. A host bind mount of nats.conf is unreadable when the container VM
# (for example a Podman machine) does not share the checkout directory, so the config is copied into the image.
# It holds no key: the JetStream key is read from $WASL_JS_KEY in the container environment.
# entrypoint.sh refuses to start on retained stores with a wrong or missing key (nats-server would otherwise start,
# fail to decrypt them and let the cluster reset the streams).
FROM nats:${NATS_VERSION}-alpine
RUN apk upgrade --no-cache
COPY --from=build /out/nats-server /usr/local/bin/nats-server
COPY nats.conf /etc/nats/wasl.conf
COPY --chmod=0755 entrypoint.sh /usr/local/bin/wasl-nats-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]
