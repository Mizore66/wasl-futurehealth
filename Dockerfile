# syntax=docker/dockerfile:1
# Wasl pipeline + API image (docs/DEPLOY.md). Multi-stage: a static Go build, then an Alpine Python runtime for the
# cohort augmentation, booking and summary scripts. Runs as a non-root user. No keys or secrets are baked in: the
# JetStream encryption key lives only in the NATS containers' environment (docker-compose.yml).

ARG GO_VERSION=1.27.3
ARG PYTHON_VERSION=3.12

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src/pipeline
COPY pipeline/go.mod pipeline/go.sum ./
RUN go mod download
COPY pipeline/ ./
RUN go vet . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wsi7 .

FROM python:${PYTHON_VERSION}-alpine
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    WASL_BIN=/app/bin WASL_OUT=/var/lib/wasl
# booking engine and scripts: numpy + scipy only; pip is not needed at runtime and is removed with its vendored SBOM
RUN apk upgrade --no-cache \
 && apk add --no-cache bash util-linux \
 && pip install --no-cache-dir --upgrade "pip==26.3" \
 && pip install --no-cache-dir "numpy==2.3.5" "scipy==1.16.3" \
 && pip uninstall -y pip \
 && rm -f /usr/local/bin/pip /usr/local/bin/pip3 /usr/local/bin/pip3.12 \
 && rm -rf /usr/local/lib/python3.12/ensurepip \
 && adduser -D -u 10001 -h /nonexistent -s /sbin/nologin wasl \
 && mkdir -p /var/lib/wasl && chown wasl:wasl /var/lib/wasl
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
VOLUME ["/var/lib/wasl"]
EXPOSE 8088
ENTRYPOINT ["/app/scripts/demo.sh"]
CMD ["container"]
