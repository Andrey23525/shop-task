FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/event-ingest

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=builder /app/config ./config

# Создаем скрипт построчно
RUN printf '#!/bin/sh\n' > /root/entrypoint.sh && \
    printf 'set -e\n' >> /root/entrypoint.sh && \
    printf 'SERVICE_DIR="/shared/events"\n' >> /root/entrypoint.sh && \
    printf 'mkdir -p "$SERVICE_DIR"\n' >> /root/entrypoint.sh && \
    printf 'cleanup() {\n' >> /root/entrypoint.sh && \
    printf '  echo "Cleaning $SERVICE_DIR ..."\n' >> /root/entrypoint.sh && \
    printf '  rm -rf "$SERVICE_DIR"/* || true\n' >> /root/entrypoint.sh && \
    printf '}\n' >> /root/entrypoint.sh && \
    printf 'trap cleanup EXIT TERM INT\n' >> /root/entrypoint.sh && \
    printf 'exec /root/main\n' >> /root/entrypoint.sh && \
    chmod +x /root/entrypoint.sh

EXPOSE 8080

CMD ["/bin/sh", "/root/entrypoint.sh"]