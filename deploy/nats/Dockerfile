# syntax=docker/dockerfile:1
# Rebuild NATS with a supported Go toolchain and patched modules: the upstream 2.12.6 image still bundles an older
# Go runtime and x/crypto. The final stage retains the official image layout and receives Alpine security updates.
ARG GO_VERSION=1.27.4
ARG NATS_VERSION=2.12.6

FROM golang:${GO_VERSION}-bookworm AS build
ARG NATS_VERSION
WORKDIR /src
ENV GOFLAGS=-mod=vendor GOPROXY=off
COPY build/ ./
ARG OFFLINE_BUILD_ID=0
RUN command -v timeout >/dev/null && command -v bash >/dev/null \
 && if [ "$OFFLINE_BUILD_ID" != "0" ] && timeout 3 bash -c 'exec 3<>/dev/tcp/proxy.golang.org/443' 2>/dev/null; then \
      echo "offline build unexpectedly reached proxy.golang.org" >&2; exit 1; \
    fi \
 && go vet . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nats-server github.com/nats-io/nats-server/v2

# NATS with the Wasl cluster config baked in. A host bind mount of nats.conf is unreadable when the container VM
# (for example a Podman machine) does not share the checkout directory, so the config is copied into the image.
# It holds no key: the JetStream key is read from $WASL_JS_KEY in the container environment.
# entrypoint.sh refuses to start on retained stores with a wrong or missing key (nats-server would otherwise start,
# fail to decrypt them and let the cluster reset the streams).
FROM --platform=linux/${TARGETARCH} nats:${NATS_VERSION}-alpine
ARG OFFLINE_PACKAGES=0
ARG TARGETARCH
COPY offline/apk/${TARGETARCH} /vendor/apk
RUN if [ "$OFFLINE_PACKAGES" = "1" ]; then \
      apk add --no-cache --no-network /vendor/apk/*.apk; \
    else \
      apk upgrade --no-cache && apk add --no-cache ca-certificates; \
    fi \
    && rm -rf /vendor \
    && addgroup -g 10001 -S wasl \
    && adduser -S -D -H -u 10001 -G wasl wasl \
    && mkdir -p /data \
    && chown -R 10001:10001 /data
COPY --from=build /out/nats-server /usr/local/bin/nats-server
COPY nats.conf /etc/nats/wasl.conf
COPY nats-production.conf /etc/nats/wasl-production.conf
COPY --chmod=0755 entrypoint.sh /usr/local/bin/wasl-nats-entrypoint.sh
RUN chown -R 10001:10001 /data /etc/nats
USER 10001:10001
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]
CMD ["nats-server", "-c", "/etc/nats/wasl.conf"]
