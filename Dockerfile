# syntax=docker/dockerfile:1
# Wasl pipeline + API image (docs/DEPLOY.md). Multi-stage: a static Go build, then an Alpine Python runtime for the
# cohort augmentation, booking and summary scripts. Runs as a non-root user. No keys or secrets are baked in: the
# JetStream encryption key lives only in the NATS containers' environment (docker-compose.yml).

ARG GO_VERSION=1.27.4
ARG PYTHON_VERSION=3.12
ARG PYTHON_IMAGE=python:${PYTHON_VERSION}-alpine

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src/pipeline
COPY pipeline/go.mod pipeline/go.sum ./
COPY pipeline/vendor ./vendor
# Dependencies are tracked in pipeline/vendor; compilation must never contact a module proxy.
ENV GOFLAGS=-mod=vendor GOPROXY=off
COPY pipeline/ ./
ARG OFFLINE_BUILD_ID=0
RUN command -v timeout >/dev/null && command -v bash >/dev/null \
 && if [ "$OFFLINE_BUILD_ID" != "0" ] && timeout 3 bash -c 'exec 3<>/dev/tcp/proxy.golang.org/443' 2>/dev/null; then \
      echo "offline build unexpectedly reached proxy.golang.org" >&2; exit 1; \
    fi \
 && go vet . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wsi7 .

FROM ${PYTHON_IMAGE}
USER root
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    WASL_BIN=/app/bin WASL_OUT=/var/lib/wasl
# Offline rebuilds start from the complete online-seeded image and skip all package downloads.
ARG OFFLINE_RUNTIME=0
RUN if [ "$OFFLINE_RUNTIME" != "1" ]; then \
      apk upgrade --no-cache \
   && apk add --no-cache bash util-linux \
   && pip install --no-cache-dir --upgrade "pip==26.3" \
   && pip install --no-cache-dir "numpy==2.3.5" "scipy==1.16.3" "cryptography==50.0.2" \
   && pip uninstall -y pip \
   && rm -f /usr/local/bin/pip /usr/local/bin/pip3 /usr/local/bin/pip3.12 \
   && rm -rf /usr/local/lib/python3.12/ensurepip \
   && adduser -D -u 10001 -h /nonexistent -s /sbin/nologin wasl \
   && mkdir -p /var/lib/wasl && chown wasl:wasl /var/lib /var/lib/wasl; \
    fi
# A reused runtime supplies packages only. Validate it, then remove every inherited app/run file before copying the checkout.
RUN if [ "$OFFLINE_RUNTIME" = "1" ]; then \
      python -c "import numpy, scipy, cryptography" \
   && command -v bash >/dev/null && command -v flock >/dev/null \
   && test "$(id -u wasl)" = 10001 \
   && mkdir -p /app /var/lib/wasl \
   && find /app /var/lib/wasl -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + \
   && chown wasl:wasl /var/lib /var/lib/wasl; \
    fi
WORKDIR /app
# synthetic cohort inputs and model weights (committed in the repo; no patient data)
COPY data/ data/
COPY models/ models/
COPY patient-parsers/econsult_testset.json patient-parsers/teachback_testset.json patient-parsers/
COPY booking/ booking/
COPY scripts/ scripts/
COPY pipeline/augment_cohort.py pipeline/
COPY pipeline/results/ltt/ltt_selection.json pipeline/results/ltt/
COPY --from=build /out/wsi7 bin/wsi7
USER 10001:10001
EXPOSE 8088
ENTRYPOINT ["/app/scripts/demo.sh"]
CMD ["container"]
