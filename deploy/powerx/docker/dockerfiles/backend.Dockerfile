FROM --platform=$BUILDPLATFORM golang:1.26.7-alpine AS builder
ARG TARGETARCH

WORKDIR /src/backend
RUN apk add --no-cache git ca-certificates

COPY backend/ ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/powerx-app ./cmd/app && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/platform_capability_seed ./cmd/platform_capability_seed

# Keep the compiled Go cache when only the release identity changes between
# acceptance attempts. The database binary still embeds the exact target SHA.
ARG POWERX_SOURCE_SHA=unknown
ARG POWERX_VERSION=unversioned
ARG POWERX_BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.commit=$POWERX_SOURCE_SHA -X main.version=$POWERX_VERSION -X main.buildTime=$POWERX_BUILD_TIME" -o /out/database ./cmd/database

FROM alpine:3.22

WORKDIR /app/backend
ENV POWERX_LINKS_ROOT=/app POWERX_MODE=docker
RUN apk add --no-cache bash ca-certificates curl tzdata python3 py3-yaml nodejs postgresql16-client && \
    mkdir -p /app/backend/config /app/backend/reports/_state /data/uploads

COPY --from=builder /out/powerx-app /app/powerx-app
COPY --from=builder /out/database /app/database
COPY --from=builder /out/platform_capability_seed /app/platform_capability_seed
COPY backend/config/ /app/backend/config/
COPY backend/scripts/ops/ /app/backend/scripts/ops/
COPY backend/api/openapi/ /app/backend/api/openapi/
COPY backend/internal/server/agent/blueprints/ /app/backend/internal/server/agent/blueprints/
COPY backend/etc/config_example.prod.yaml /app/container/config.example.yaml
COPY deploy/powerx/container/init.py /app/container/init.py
COPY deploy/powerx/container/upgrade_config.py /app/container/upgrade_config.py
COPY deploy/powerx/container/entrypoint.sh /app/container/entrypoint.sh
RUN chmod 755 /app/container/entrypoint.sh && ln -s /app/backend/config /app/config

EXPOSE 8080
ENTRYPOINT ["/app/container/entrypoint.sh"]
CMD ["serve"]
