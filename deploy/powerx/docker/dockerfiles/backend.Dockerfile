FROM golang:1.24-alpine AS builder

WORKDIR /src/backend
RUN apk add --no-cache git ca-certificates

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/powerx-app ./cmd/app && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/database ./cmd/database

FROM alpine:3.20

WORKDIR /app
RUN apk add --no-cache ca-certificates curl tzdata && \
    mkdir -p /app/backend/config /app/backend/reports/_state /data/uploads

COPY --from=builder /out/powerx-app /app/powerx-app
COPY --from=builder /out/database /app/database
COPY backend/config/ /app/backend/config/

EXPOSE 8080
CMD ["./powerx-app"]
