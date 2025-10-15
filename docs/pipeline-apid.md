## Pipeline API — спецификация

Сервис `pipeline-api` отвечает за запись и выдачу статусов движения задач по пайплайну на основе таблицы `pipeline_tracking` (отдельная БД пайплайна, см. примечания ниже). Реализация демона не требуется; ниже описан контракт API и обязанности демонов (producers/consumers) по изменению статусов.

### Базовые сущности
- `filename` — префикс имен файла событий до `.event-ingest.txt` (например: `20250101000000`).
- `shard` — номер шарда (целое число, не NULL). Запись создается на каждый шард.
- Статусы на этапах: `new`, `started`, `done`, `failed`.
- Этапы: `event-ingest`, `shard-splitter`, `shard-worker`, `mysql-sender`.

### Правила и инварианты
- Для каждого нового файла событий создаются записи в `pipeline_tracking` по количеству шардов.
- Переход статуса на каждом этапе следует схеме: `new -> started -> done` или `new/started -> failed`.
- Повторная постановка в обработку: `failed -> new`.
- Идемпотентность: операции идентифицируются парой `(filename, shard)`.
- Шард обязателен на всех этапах, кроме регистрации файла (где он задается списком шардов).

### Обязанности демонов
- Event Ingest (производитель файлов):
  - Регистрирует файл и создает записи по шардам.
  - Отмечает окончание генерации файла (`event_ingest_status=done`).
- Shard Splitter:
  - Берет записи после `event_ingest_status='done'` (готовые к этапу shard-splitter).
  - Помечает `started`/`done`/`failed` для `shard_splitter_status`.
- Shard Worker:
  - Берет записи с `shard_splitter_status=done` и обрабатывает их.
  - Помечает `started`/`done`/`failed` для `shard_worker_status`.
- MySQL Sender:
  - Берет записи с `shard_worker_status=done` и отправляет в нужный шард БД.
  - Помечает `started`/`done`/`failed` для `mysql_sender_status`.

---

## Эндпоинты

Базовый URL: `/api/v1/pipeline`

### 1) Регистрация файла и создание записей по шардам
POST `/files/register`

Request:
```json
{
  "filename": "20250101000000",
  "shards": [0, 1]
}
```

Действия:
- Если записей нет — создать по одной на каждый `shard` со статусами:
  - `event_ingest_status = 'started'`
  - остальные статусы = `'new'`
- Если записи уже существуют — ответить успешно (идемпотентно).

Response 200:
```json
{ "filename": "20250101000000", "shards": [0,1], "created": true }
```

Пример SQL (идемпотентная вставка):
```sql
INSERT INTO pipeline_tracking (filename, shard, event_ingest_status)
VALUES (:filename, :shard, 'started')
ON DUPLICATE KEY UPDATE filename = filename;
```

### 2) Пометить завершение генерации файла (event-ingest done)
POST `/stages/event-ingest/done`

Request:
```json
{ "filename": "20250101000000" }
```

Действия:
- Для всех shards данного файла: `event_ingest_status = 'done'`.

Response 200:
```json
{ "filename": "20250101000000", "updated_rows": 2 }
```

SQL:
```sql
UPDATE pipeline_tracking
SET event_ingest_status = 'done', updated_at = NOW()
WHERE filename = :filename;
```

### 3) Универсальные переходы статусов для этапов
POST `/stages/{stage}/{transition}`

Path params:
- `stage` ∈ {`shard-splitter`, `shard-worker`, `mysql-sender`}
- `transition` ∈ {`start`, `done`, `fail`}

Request:
```json
{ "filename": "20250101000000", "shard": 0 }
```

Переходы:
- `start`  → статус `{stage}_status = 'started'`
- `done`   → статус `{stage}_status = 'done'`
- `fail`   → статус `{stage}_status = 'failed'`

Response 200:
```json
{ "filename": "20250101000000", "shard": 0, "stage": "shard-worker", "status": "done" }
```

Примеры SQL:
```sql
-- start
UPDATE pipeline_tracking
SET shard_worker_status = 'started', updated_at = NOW()
WHERE filename = :filename AND shard = :shard;

-- done
UPDATE pipeline_tracking
SET shard_worker_status = 'done', updated_at = NOW()
WHERE filename = :filename AND shard = :shard;

-- fail
UPDATE pipeline_tracking
SET shard_worker_status = 'failed', updated_at = NOW()
WHERE filename = :filename AND shard = :shard;
```

### 4) Сброс статуса с failed на new (ретрай)
POST `/stages/{stage}/retry`

Request:
```json
{ "filename": "20250101000000", "shard": 1 }
```

Действия:
- `{stage}_status = 'new'` только если текущий статус `'failed'`.

Response 200:
```json
{ "filename": "20250101000000", "shard": 1, "stage": "mysql-sender", "status": "new" }
```

SQL:
```sql
UPDATE pipeline_tracking
SET mysql_sender_status = 'new', updated_at = NOW()
WHERE filename = :filename AND shard = :shard AND mysql_sender_status = 'failed';
```

### 5) Очереди для демонов (pull-модель)
GET `/queues/{stage}`

Purpose: выдать список задач, готовых к этапу `stage`.

Правила готовности:
- Для `shard-splitter`: `event_ingest_status='done' AND shard_splitter_status='new'`
- Для `shard-worker`: `shard_splitter_status='done' AND shard_worker_status='new'`
- Для `mysql-sender`: `shard_worker_status='done' AND mysql_sender_status='new'`

Response 200:
```json
{
  "stage": "shard-worker",
  "items": [
    { "filename": "20250101000000", "shard": 0, "created_at": "2025-01-01T00:00:00Z" }
  ]
}
```

Примеры SQL:
```sql
SELECT filename, shard, created_at
FROM pipeline_tracking
WHERE shard_splitter_status = 'done' AND shard_worker_status = 'new'
ORDER BY created_at
LIMIT :limit OFFSET :offset;
```

### 6) Получить статусы по файлу
GET `/files/{filename}`

Response 200:
```json
{
  "filename": "20250101000000",
  "shards": [
    {
      "shard": 0,
      "event_ingest_status": "done",
      "shard_splitter_status": "done",
      "shard_worker_status": "started",
      "mysql_sender_status": "new",
      "updated_at": "2025-01-01T00:05:00Z"
    }
  ]
}
```

SQL:
```sql
SELECT shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, updated_at
FROM pipeline_tracking
WHERE filename = :filename
ORDER BY shard;
```

### 7) Метрики/сводка по этапам
GET `/metrics/summary`

Response 200:
```json
{
  "total": 120,
  "by_stage": {
    "shard_worker": { "new": 10, "started": 5, "done": 100, "failed": 5 }
  },
  "by_shard": [
    { "shard": 0, "total": 60, "done": 50, "failed": 2 },
    { "shard": 1, "total": 60, "done": 50, "failed": 3 }
  ]
}
```

Примеры SQL (фрагменты):
```sql
SELECT COUNT(*) AS total FROM pipeline_tracking;

SELECT 
  SUM(shard_worker_status='new')     AS new_cnt,
  SUM(shard_worker_status='started') AS started_cnt,
  SUM(shard_worker_status='done')    AS done_cnt,
  SUM(shard_worker_status='failed')  AS failed_cnt
FROM pipeline_tracking;

SELECT shard,
  COUNT(*) AS total,
  SUM(mysql_sender_status='done')   AS done_cnt,
  SUM(mysql_sender_status='failed') AS failed_cnt
FROM pipeline_tracking
GROUP BY shard
ORDER BY shard;
```

---

## Сценарии использования (пример)

1) Event Ingest сгенерировал файл `20250101000000.event-ingest.txt` для шардов `[0,1]`:
   - POST `/files/register` → создаем 2 записи (`shard=0,1`, `event_ingest_status='started'`).
   - POST `/stages/event-ingest/done` → отмечаем завершение генерации.

2) Shard Splitter:
   - GET `/queues/shard-splitter` → берет задачи `new` с `event_ingest_status='done'`.
   - POST `/stages/shard-splitter/start` (по каждой задаче).
   - POST `/stages/shard-splitter/done` (по завершении).

3) Shard Worker:
   - GET `/queues/shard-worker` → берет задачи `new` с `shard_splitter_status='done'`.
   - POST `/stages/shard-worker/start` → потом `/done` или `/fail`.

4) MySQL Sender:
   - GET `/queues/mysql-sender` → берет задачи `new` с `shard_worker_status='done'`.
   - POST `/stages/mysql-sender/start` → потом `/done` или `/fail`.
   - При падении — POST `/stages/mysql-sender/retry` для возврата в `new`.

---

## Замечания по реализации
- Таблица `pipeline_tracking` должна жить в отдельной БД (например, `pipeline_db`), не в шардах приложений.
- Вынесите её DDL в отдельный файл, например `db/pipeline_ddl.sql`, и применяйте при старте `pipeline-apid`.
- Рекомендуется уникальный ключ `(filename, shard)` для идемпотентности регистрации файла:
  ```sql
  ALTER TABLE pipeline_tracking ADD UNIQUE KEY uq_filename_shard (filename, shard);
  ```
- Для конкурентной выборки очередей используйте транзакции/блокировки или атомарные обновления со статусом `started`.
- Ограничивайте размер выдачи очередей (`limit`, `offset`) и фильтруйте по `updated_at`.
