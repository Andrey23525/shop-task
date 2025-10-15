# Event-Driven Shop Management System

Событийно-ориентированная система учета товаров, магазинов, пользователей и операций.

## Архитектура

Система состоит из следующих компонентов:

- **Event Ingest** - генерация событий и запись в текстовые файлы
- **Shard Splitter** - маршрутизация событий по шардам
- **Shard Workers** - обработка событий по шардам
- **Balance Daemon** - управление балансами пользователей
- **Cache Service** - кеширование ожидающих событий
- **pipeline-apid** - API для трекинга задач пайплайна (таблица `pipeline_tracking`)

## Шардирование

Система использует шардирование по `shop_id % 2`:
- Шард 0: четные shop_id
- Шард 1: нечетные shop_id

## Типы событий

| Тип | Назначение | Ключ упорядочивания |
|-----|------------|-------------------|
| 0 | Изменение количества | shop_id |
| 1 | Покупка/продажа | order_id |
| 2 | Изменение цены | shop_id |
| 3 | Возврат | order_id |
| 4 | Открытие/закрытие магазина | shop_id |

## Запуск

```bash
docker-compose up -d
```

## Event Ingest

Сервис автоматически генерирует события каждые `generation_interval` секунд и пишет их в текстовые файлы (`/shared/events/`).

Документация: `docs/event-ingest.md`

Ключевое:
- Имя файла: `YYYYMMDDHHMMSS.event-ingest.txt`
- По одному событию в строке (JSON)
- Фактический размер батча: `min(events_per_batch, max_batch_size)`
- Интеграция: уведомляет `pipeline-apid` о регистрации файла и `done`

Конфигурация (TOML): `services/event-ingest/config/app.toml`

HTTP:
- `GET /health`, `GET /ready`, `GET /api/v1/events/stats`

## Тестирование

```bash
# Запуск тестов
make test

# Нагрузочное тестирование
make load-test
```

## Быстрый старт (docker compose)

```bash
# Поднять весь стек (шарды MySQL, mysql-pipeline, event-ingest и пр.)
docker compose -f docker-compose.yml up -d --build

# Проверить, что сервисы поднялись
docker compose ps

# Посмотреть логи event-ingest
docker compose logs -f event-ingest

# Список сгенерированных файлов
ls -l ./shared/events

# Проверка MySQL пайплайна (pipeline_db)
docker exec -it shop-mysql-pipeline mysql -uroot -prootpassword -e "SHOW DATABASES; USE pipeline_db; SHOW TABLES; SELECT COUNT(*) FROM pipeline_tracking;"
```

Остановка и очистка (удалит тома, включая данные MySQL):
```bash
docker compose -f docker-compose.yml down -v
```

## Полезные документы
- Документация по Event Ingest: `docs/event-ingest.md`
- Спецификация pipeline-apid: `docs/pipeline-apid.md`
- Задание (реализация демонов): `docs/student-assignment.md`
