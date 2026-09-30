# Build context là thư mục gốc của repo (cần cả backend/ lẫn frontend/):
#   docker build -f backend/Dockerfile -t awning-backend .
# Hoặc dùng deploy/docker-compose.yml.

FROM golang:1.24-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Không cgo (SQLite là bản thuần Go) nên chạy được trên image trống.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && mkdir /data-dir

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build --chown=65532:65532 /data-dir /data
COPY frontend/ /app/frontend/
ENV HTTP_ADDR=:8080 \
    STATIC_DIR=/app/frontend \
    DB_PATH=/data/awning.db
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/server", "-healthcheck"]
ENTRYPOINT ["/app/server"]
