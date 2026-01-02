FROM rust:alpine as builder
WORKDIR /app
RUN apk add --no-cache musl-dev

# Кэшируем зависимости
COPY Cargo.toml Cargo.lock ./
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    mkdir src && \
    echo "fn main() {}" > src/main.rs && \
    cargo build --release && \
    rm -rf src

# Собираем реальный код
COPY src ./src
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/app/target \
    cargo build --release && \
    cp /app/target/release/shard-splitter /usr/local/bin/

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache libgcc
RUN apk --no-cache add ca-certificates bash
COPY --from=builder /usr/local/bin/shard-splitter ./main
# Копируем бинарный файл из builder stage (исправлен путь и имя)
#COPY --from=builder /app/target/release/shard-splitter ./main

# Создаём скрипт запуска
RUN printf '#!/usr/bin/env bash\n' > /app/run.sh && \
    printf 'set -e\n' >> /app/run.sh && \
    printf 'SERVICE_DIR="/shared/pipeline-api"\n' >> /app/run.sh && \
    printf 'mkdir -p "$SERVICE_DIR"\n' >> /app/run.sh && \
    printf 'cleanup(){ echo "Cleaning $SERVICE_DIR ..."; rm -rf "$SERVICE_DIR"/* || true; }\n' >> /app/run.sh && \
    printf 'trap cleanup EXIT TERM INT\n' >> /app/run.sh && \
    printf 'echo "Pipeline API Service"\n' >> /app/run.sh && \
    printf 'echo "Starting..."\n' >> /app/run.sh && \
    printf './main | tee -a "$SERVICE_DIR/activity.log"\n' >> /app/run.sh && \
    chmod +x /app/run.sh

EXPOSE 8082

CMD ["/app/run.sh"]