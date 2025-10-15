## Event Ingest — описание сервиса

Сервис `event-ingest` автономно генерирует события и пишет их в текстовые файлы в общей директории. Он не шардирует и не пишет в БД — только формирует файлы событий и уведомляет `pipeline-apid` о появлении нового файла.

### Назначение
- Периодически (по расписанию) генерировать события разных типов.
- Каждые N секунд создавать файл с именем по шаблону: `YYYYMMDDHHMMSS.event-ingest.txt` и писать в него по одной JSON‑строке на событие.
- Отправлять в `pipeline-apid` два запроса: регистрация файла по всем шардам и пометка этапа `event-ingest` как `done`.

### Формат записи события
Каждая строка — валидный JSON:
```json
{
  "event_type": 0,
  "payload": { ... },
  "saved_at": "2025-10-15 17:41:40.018",
  "timestamp": "2025-10-15 17:41:40.021"
}
```

Поддерживаемые типы событий (event_type):
- 0: Restock (пополнение/списание остатков)
- 1: Purchase (покупка/продажа, содержит `order_id` — int64)
- 2: Price Change (изменение цены)
- 3: Return (возврат, содержит `order_id` — int64)
- 4: Shop Status (изменение статуса магазина)

### Конфигурация (TOML)
Файл: `services/event-ingest/config/app.toml`
```toml
[server]
host = "0.0.0.0"
port = 8080
read_timeout = 30
write_timeout = 30
idle_timeout = 120

[events]
max_payload_size = 1048576
validation_timeout = 5
max_batch_size = 1000        # верхний лимит (гард) на размер батча
generation_interval = 10     # интервал генерации (сек)
events_per_batch = 5         # фактический максимум на тик; min(events_per_batch, max_batch_size)

[logging]
level = "INFO"
format = "json"

[pipeline]
url = "http://pipeline-apid:8082/api/v1/pipeline"
shards_count = 2
```

Переопределения через env:
- `SERVER_PORT`
- `PIPELINE_URL`
- `PIPELINE_SHARDS_COUNT`

### Расписание и синхронизация по границам
- Сервис ждет ближайшую границу `generation_interval` (например, кратно 10 сек), затем каждые `generation_interval` запускает один тик генерации.
- На каждом тике создается новый файл `YYYYMMDDHHMMSS.event-ingest.txt` (префикс — момент тика).

### Размер батча
- На каждом тике генерируется случайное число событий от 1 до `min(events_per_batch, max_batch_size)`.
- Для увеличения нагрузки — повышайте `events_per_batch` и/или уменьшайте `generation_interval`.

### Директории и очистка
- Общая директория, смонтированная во все контейнеры: `/shared`.
- `event-ingest` пишет файлы в поддиректорию: `/shared/events`.
- Каждый демон пишет в свою поддиректорию:
  - `shard-splitter`: `/shared/shard-splitter`
  - `shard-worker-$SHARD_ID`: `/shared/shard-worker-<id>`
  - `balance-daemon`: `/shared/balance-daemon`
  - `cache-service`: `/shared/cache-service`
- При остановке контейнера соответствующая поддиректория очищается (trap EXIT/TERM/INT).

### Интеграция с Pipeline API
На каждый файл `prefix = YYYYMMDDHHMMSS` выполняются запросы:
1) Регистрация файла по шардам
```
POST {pipeline.url}/files/register
{
  "filename": "<prefix>",
  "shards": [0, 1, ..., shards_count-1]
}
```
2) Пометка завершения этапа event-ingest
```
POST {pipeline.url}/stages/event-ingest/done
{ "filename": "<prefix>" }
```
Ошибки интеграции не блокируют запись файла (best‑effort). Таймаут HTTP‑клиента: 5 сек.

### HTTP API `event-ingest`
Минимальный, для технических проверок:
- `GET /health` — состояние
- `GET /ready` — готовность
- `GET /api/v1/events/stats` — краткая статистика по файлам/директории

### Сборка и запуск
- Образ: многостадийный Dockerfile (Alpine + Golang builder).
- Точки монтирования: `/shared` (общая), `./config` (внутри образа).
- Логи — стандартный stdout контейнера.

### Ограничения и допущения
- Сервис не валидирует бизнес‑сущности в БД и не пишет в MySQL.
- Идемпотентность событий обеспечивается на уровне потребителей.
- Формат временных полей: `YYYY-MM-DD HH:MM:SS.MS` (UTC).
