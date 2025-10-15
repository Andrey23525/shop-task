-- Примеры запросов для отслеживания движения задач по пайплайну
-- Все запросы работают с таблицей pipeline_tracking из ddl.sql

-- 1. Создание новых задач при генерации файла event-ingest
-- Event-ingest создает записи для каждого шарда (0 и 1)
INSERT INTO pipeline_tracking (filename, event_ingest_status, shard) 
VALUES 
    ('20250101000000', 'started', 0),
    ('20250101000000', 'started', 1);

-- Проверяем состояние после создания записей
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 2. Обновление статуса event-ingest на done для всех шардов
UPDATE pipeline_tracking 
SET event_ingest_status = 'done', updated_at = NOW() 
WHERE filename = '20250101000000';

-- Проверяем состояние после завершения event-ingest
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 3. Shard-splitter начинает обработку файла для шарда 0
UPDATE pipeline_tracking 
SET shard_splitter_status = 'started', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0 AND event_ingest_status = 'done';

-- Проверяем состояние после начала shard-splitter для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 4. Shard-splitter завершает обработку для шарда 0
UPDATE pipeline_tracking 
SET shard_splitter_status = 'done', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0;

-- Проверяем состояние после завершения shard-splitter для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 5. Shard-worker начинает обработку для шарда 0
UPDATE pipeline_tracking 
SET shard_worker_status = 'started', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0 AND shard_splitter_status = 'done';

-- Проверяем состояние после начала shard-worker для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 6. Shard-worker завершает обработку для шарда 0
UPDATE pipeline_tracking 
SET shard_worker_status = 'done', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0;

-- Проверяем состояние после завершения shard-worker для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 7. MySQL-sender начинает отправку данных для шарда 0
UPDATE pipeline_tracking 
SET mysql_sender_status = 'started', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0 AND shard_worker_status = 'done';

-- Проверяем состояние после начала mysql-sender для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- 8. MySQL-sender завершает отправку для шарда 0
UPDATE pipeline_tracking 
SET mysql_sender_status = 'done', updated_at = NOW() 
WHERE filename = '20250101000000' AND shard = 0;

-- Проверяем финальное состояние после завершения mysql-sender для шарда 0
SELECT filename, shard, event_ingest_status, shard_splitter_status, shard_worker_status, mysql_sender_status, created_at, updated_at 
FROM pipeline_tracking 
WHERE filename = '20250101000000' 
ORDER BY shard;

-- ===========================================
-- ЗАПРОСЫ ДЛЯ МОНИТОРИНГА
-- ===========================================

-- 9. Показать все задачи в процессе обработки
SELECT 
    filename,
    event_ingest_status,
    shard_splitter_status,
    shard_worker_status,
    mysql_sender_status,
    shard,
    created_at,
    updated_at
FROM pipeline_tracking 
WHERE event_ingest_status != 'done' 
   OR shard_splitter_status != 'done' 
   OR shard_worker_status != 'done' 
   OR mysql_sender_status != 'done'
ORDER BY created_at DESC;

-- 10. Показать задачи, застрявшие на определенном этапе
SELECT 
    filename,
    event_ingest_status,
    shard_splitter_status,
    shard_worker_status,
    mysql_sender_status,
    shard,
    created_at,
    updated_at,
    TIMESTAMPDIFF(MINUTE, updated_at, NOW()) as stuck_minutes
FROM pipeline_tracking 
WHERE (
    (event_ingest_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 5) OR
    (shard_splitter_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 3) OR
    (shard_worker_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 10) OR
    (mysql_sender_status = 'started' AND TIMESTAMPDIFF(MINUTE, updated_at, NOW()) > 5)
)
ORDER BY stuck_minutes DESC;

-- 11. Статистика по статусам
SELECT 
    'event_ingest' as service,
    event_ingest_status as status,
    COUNT(*) as count
FROM pipeline_tracking 
GROUP BY event_ingest_status
UNION ALL
SELECT 
    'shard_splitter' as service,
    shard_splitter_status as status,
    COUNT(*) as count
FROM pipeline_tracking 
GROUP BY shard_splitter_status
UNION ALL
SELECT 
    'shard_worker' as service,
    shard_worker_status as status,
    COUNT(*) as count
FROM pipeline_tracking 
GROUP BY shard_worker_status
UNION ALL
SELECT 
    'mysql_sender' as service,
    mysql_sender_status as status,
    COUNT(*) as count
FROM pipeline_tracking 
GROUP BY mysql_sender_status
ORDER BY service, status;

-- 12. Показать полностью обработанные задачи за последний час
SELECT 
    filename,
    shard,
    created_at,
    updated_at,
    TIMESTAMPDIFF(SECOND, created_at, updated_at) as processing_time_seconds
FROM pipeline_tracking 
WHERE event_ingest_status = 'done' 
  AND shard_splitter_status = 'done' 
  AND shard_worker_status = 'done' 
  AND mysql_sender_status = 'done'
  AND created_at >= DATE_SUB(NOW(), INTERVAL 1 HOUR)
ORDER BY created_at DESC;

-- 13. Показать задачи с ошибками
SELECT 
    filename,
    event_ingest_status,
    shard_splitter_status,
    shard_worker_status,
    mysql_sender_status,
    shard,
    created_at,
    updated_at
FROM pipeline_tracking 
WHERE event_ingest_status = 'failed' 
   OR shard_splitter_status = 'failed' 
   OR shard_worker_status = 'failed' 
   OR mysql_sender_status = 'failed'
ORDER BY updated_at DESC;

-- 14. Показать задачи по шардам
SELECT 
    shard,
    COUNT(*) as total_tasks,
    SUM(CASE WHEN mysql_sender_status = 'done' THEN 1 ELSE 0 END) as completed_tasks,
    SUM(CASE WHEN mysql_sender_status = 'started' THEN 1 ELSE 0 END) as processing_tasks,
    SUM(CASE WHEN mysql_sender_status = 'failed' THEN 1 ELSE 0 END) as failed_tasks
FROM pipeline_tracking 
GROUP BY shard
ORDER BY shard;

-- 15. Показать среднее время обработки по этапам
SELECT 
    'event_ingest' as stage,
    AVG(CASE 
        WHEN event_ingest_status = 'done' 
        THEN TIMESTAMPDIFF(SECOND, created_at, updated_at) 
        ELSE NULL 
    END) as avg_processing_time_seconds
FROM pipeline_tracking 
WHERE event_ingest_status IN ('done', 'failed')
UNION ALL
SELECT 
    'shard_splitter' as stage,
    AVG(CASE 
        WHEN shard_splitter_status = 'done' 
        THEN TIMESTAMPDIFF(SECOND, created_at, updated_at) 
        ELSE NULL 
    END) as avg_processing_time_seconds
FROM pipeline_tracking 
WHERE shard_splitter_status IN ('done', 'failed')
UNION ALL
SELECT 
    'shard_worker' as stage,
    AVG(CASE 
        WHEN shard_worker_status = 'done' 
        THEN TIMESTAMPDIFF(SECOND, created_at, updated_at) 
        ELSE NULL 
    END) as avg_processing_time_seconds
FROM pipeline_tracking 
WHERE shard_worker_status IN ('done', 'failed')
UNION ALL
SELECT 
    'mysql_sender' as stage,
    AVG(CASE 
        WHEN mysql_sender_status = 'done' 
        THEN TIMESTAMPDIFF(SECOND, created_at, updated_at) 
        ELSE NULL 
    END) as avg_processing_time_seconds
FROM pipeline_tracking 
WHERE mysql_sender_status IN ('done', 'failed');

-- ===========================================
-- ДОПОЛНИТЕЛЬНЫЕ ЗАПРОСЫ ДЛЯ РАБОТЫ С ШАРДАМИ
-- ===========================================

-- 16. Показать задачи для конкретного шарда
SELECT 
    filename,
    event_ingest_status,
    shard_splitter_status,
    shard_worker_status,
    mysql_sender_status,
    created_at,
    updated_at
FROM pipeline_tracking 
WHERE shard = 0
ORDER BY created_at DESC;

-- 17. Показать задачи для конкретного файла по всем шардам
SELECT 
    shard,
    event_ingest_status,
    shard_splitter_status,
    shard_worker_status,
    mysql_sender_status,
    created_at,
    updated_at
FROM pipeline_tracking 
WHERE filename = '20250101000000'
ORDER BY shard;

-- 18. Показать задачи, готовые для обработки в shard-splitter
SELECT 
    filename,
    shard,
    created_at
FROM pipeline_tracking 
WHERE event_ingest_status = 'done' 
  AND shard_splitter_status = 'new'
ORDER BY created_at;

-- 19. Показать задачи, готовые для обработки в shard-worker
SELECT 
    filename,
    shard,
    created_at
FROM pipeline_tracking 
WHERE shard_splitter_status = 'done' 
  AND shard_worker_status = 'new'
ORDER BY created_at;

-- 20. Показать задачи, готовые для отправки в MySQL
SELECT 
    filename,
    shard,
    created_at
FROM pipeline_tracking 
WHERE shard_worker_status = 'done' 
  AND mysql_sender_status = 'new'
ORDER BY created_at;
