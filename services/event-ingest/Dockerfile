FROM golang:1.21-alpine AS builder

WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Синхронизируем зависимости и go.sum
RUN go mod tidy

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/event-ingest

FROM alpine:latest

RUN apk --no-cache add ca-certificates bash
WORKDIR /root/

# Копируем бинарный файл
COPY --from=builder /app/main .

# Копируем конфигурацию
COPY --from=builder /app/config ./config

# Обертка для подготовки директории и очистки при остановке
RUN echo '#!/usr/bin/env bash\n\nset -e\nSERVICE_DIR="/shared/events"\nmkdir -p "$SERVICE_DIR"\ncleanup() {\n  echo "Cleaning $SERVICE_DIR ..."\n  rm -rf "$SERVICE_DIR"/* || true\n}\ntrap cleanup EXIT TERM INT\nexec /root/main\n' > /root/entrypoint.sh && chmod +x /root/entrypoint.sh

EXPOSE 8080

CMD ["/root/entrypoint.sh"]
