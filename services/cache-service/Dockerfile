FROM alpine:latest

RUN apk --no-cache add curl bash

WORKDIR /app

# Скрипт сервиса
RUN echo '#!/usr/bin/env bash\nset -e\nSERVICE_DIR="/shared/cache-service"\nmkdir -p "$SERVICE_DIR"\ncleanup(){ echo "Cleaning $SERVICE_DIR ..."; rm -rf "$SERVICE_DIR"/* || true; }\ntrap cleanup EXIT TERM INT\necho "Cache Service"\necho "Starting..."\nwhile true; do\n  echo "Processing cache operations..." | tee -a "$SERVICE_DIR/activity.log"\n  sleep 30\ndone\n' > /app/run.sh && chmod +x /app/run.sh

EXPOSE 8080

CMD ["/app/run.sh"]
