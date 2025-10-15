-- Pipeline DB schema (separate database for pipeline-apid)
CREATE DATABASE IF NOT EXISTS pipeline_db
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_unicode_ci;

USE pipeline_db;

-- Tracking table for pipeline task movement
CREATE TABLE IF NOT EXISTS pipeline_tracking (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    filename VARCHAR(255) NOT NULL COMMENT 'Префикс файла до .event-ingest.txt (например: 20250101000000)',
    event_ingest_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new' COMMENT 'Статус обработки в event-ingest',
    shard_splitter_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new' COMMENT 'Статус обработки в shard-splitter',
    shard_worker_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new' COMMENT 'Статус обработки в shard-worker',
    mysql_sender_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new' COMMENT 'Статус отправки в MySQL',
    shard TINYINT NOT NULL COMMENT 'Номер шарда (0 или 1)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT 'Время создания записи',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'Время последнего обновления',

    -- Индексы для быстрого поиска
    UNIQUE KEY uq_filename_shard (filename, shard),
    INDEX idx_event_ingest_status (event_ingest_status),
    INDEX idx_shard_splitter_status (shard_splitter_status),
    INDEX idx_shard_worker_status (shard_worker_status),
    INDEX idx_mysql_sender_status (mysql_sender_status),
    INDEX idx_shard (shard),
    INDEX idx_created_at (created_at),
    INDEX idx_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Отслеживание движения задач по пайплайну';
