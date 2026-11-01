# syntax=docker/dockerfile:1
# Wasl pipeline + API image (docs/DEPLOY.md). Multi-stage: a static Go build, then a slim Python runtime for the
# cohort augmentation, booking and summary scripts. Runs as a non-root user. No keys or secrets are baked in: the
# JetStream encryption key lives only in the NATS containers' environment (docker-compose.yml).

ARG GO_VERSION=1.27
ARG PYTHON_VERSION=3.12

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src/pipeline
COPY pipeline/go.mod pipeline/go.sum ./
RUN go mod download
COPY pipeline/ ./
RUN go vet . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wsi7 .

FROM python:${PYTHON_VERSION}-slim-bookworm
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    WASL_BIN=/app/bin WASL_OUT=/var/lib/wasl
# booking engine and scripts: numpy + scipy only
RUN pip install --no-cache-dir "numpy==2.3.5" "scipy==1.16.3" \
 && useradd --uid 10001 --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin wasl \
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
