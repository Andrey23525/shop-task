# Пайплайн обработки событий

## Схема движения задач

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Event Ingest  │───▶│  Shard Splitter  │───▶│  Shard Worker   │───▶│  MySQL Sender   │
│                 │    │                  │    │                 │    │                 │
│ Status:         │    │ Status:          │    │ Status:         │    │ Status:         │
│ • new           │    │ • new            │    │ • new           │    │ • new           │
│ • started       │    │ • started        │    │ • started       │    │ • started       │
│ • done          │    │ • done           │    │ • done          │    │ • done          │
│ • failed        │    │ • failed         │    │ • failed        │    │ • failed        │
└─────────────────┘    └──────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │                       │
         ▼                       ▼                       ▼                       ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Генерирует     │    │  Определяет      │    │  Обрабатывает   │    │  Сохраняет в    │
│  файл событий   │    │  шард по         │    │  события по     │    │  MySQL шард     │
│  каждые 10 сек  │    │  shop_id % 2     │    │  определенному  │    │  (0 или 1)      │
│                 │    │                  │    │  шарду          │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘    └─────────────────┘
```

## Статусы задач

### Event Ingest
- **new** - файл еще не создан
- **started** - файл создается/записывается
- **done** - файл готов к обработке
- **failed** - ошибка при создании файла

### Shard Splitter
- **new** - файл еще не обработан
- **started** - файл читается, определяется шард
- **done** - шард определен, файл передан worker'у
- **failed** - ошибка при определении шарда

### Shard Worker
- **new** - события еще не обработаны
- **started** - события обрабатываются
- **done** - события готовы к отправке в MySQL
- **failed** - ошибка при обработке событий

### MySQL Sender
- **new** - данные еще не отправлены
- **started** - данные отправляются в MySQL
- **done** - данные успешно сохранены в MySQL
- **failed** - ошибка при сохранении в MySQL

## Пример движения задачи

### 1. Event Ingest создает файл и записи для каждого шарда
```sql
INSERT INTO pipeline_tracking (filename, event_ingest_status, shard) 
VALUES 
    ('20250101000000', 'started', 0),
    ('20250101000000', 'started', 1);
```

### 2. Event Ingest завершает создание файла для всех шардов
```sql
UPDATE pipeline_tracking 
SET event_ingest_status = 'done' 
WHERE filename = '20250101000000';
```

### 3. Shard Splitter начинает обработку для шарда 0
```sql
UPDATE pipeline_tracking 
SET shard_splitter_status = 'started' 
WHERE filename = '20250101000000' AND shard = 0 AND event_ingest_status = 'done';
```

### 4. Shard Splitter завершает обработку для шарда 0
```sql
UPDATE pipeline_tracking 
SET shard_splitter_status = 'done' 
WHERE filename = '20250101000000' AND shard = 0;
```

### 5. Shard Worker начинает обработку для шарда 0
```sql
UPDATE pipeline_tracking 
SET shard_worker_status = 'started' 
WHERE filename = '20250101000000' AND shard = 0 AND shard_splitter_status = 'done';
```

### 6. Shard Worker завершает обработку для шарда 0
```sql
UPDATE pipeline_tracking 
SET shard_worker_status = 'done' 
WHERE filename = '20250101000000' AND shard = 0;
```

### 7. MySQL Sender начинает отправку для шарда 0
```sql
UPDATE pipeline_tracking 
SET mysql_sender_status = 'started' 
WHERE filename = '20250101000000' AND shard = 0 AND shard_worker_status = 'done';
```

### 8. MySQL Sender завершает отправку для шарда 0
```sql
UPDATE pipeline_tracking 
SET mysql_sender_status = 'done' 
WHERE filename = '20250101000000' AND shard = 0;
```

## Мониторинг пайплайна

### Задачи в процессе
```sql
SELECT filename, event_ingest_status, shard_splitter_status, 
       shard_worker_status, mysql_sender_status, shard
FROM pipeline_tracking 
WHERE event_ingest_status != 'done' 
   OR shard_splitter_status != 'done' 
   OR shard_worker_status != 'done' 
   OR mysql_sender_status != 'done';
```

### Застрявшие задачи
```sql
SELECT filename, event_ingest_status, shard_splitter_status, 
       shard_worker_status, mysql_sender_status,
       TIMESTAMPDIFF(MINUTE, updated_at, NOW()) as stuck_minutes
FROM pipeline_tracking 
WHERE (event_ingest_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 5)
   OR (shard_splitter_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 3)
   OR (shard_worker_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 10)
   OR (mysql_sender_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 5);
```

### Статистика по шардам
```sql
SELECT shard, COUNT(*) as total_tasks,
       SUM(CASE WHEN mysql_sender_status = 'done' THEN 1 ELSE 0 END) as completed,
       SUM(CASE WHEN mysql_sender_status = 'started' THEN 1 ELSE 0 END) as processing,
       SUM(CASE WHEN mysql_sender_status = 'failed' THEN 1 ELSE 0 END) as failed
FROM pipeline_tracking 
GROUP BY shard;
```
