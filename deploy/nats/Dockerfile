# syntax=docker/dockerfile:1
# NATS with the Wasl cluster config baked in. A host bind mount of nats.conf is unreadable when the container VM
# (for example a Podman machine) does not share the checkout directory, so the config is copied into the image.
# It holds no key: the JetStream key is read from $WASL_JS_KEY in the container environment.
# entrypoint.sh refuses to start on retained stores with a wrong or missing key (nats-server would otherwise start,
# fail to decrypt them and let the cluster reset the streams).
FROM nats:2.12.1-alpine
COPY nats.conf /etc/nats/wasl.conf
COPY --chmod=0755 entrypoint.sh /usr/local/bin/wasl-nats-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]
