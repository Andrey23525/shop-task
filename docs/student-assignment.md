## Задания для реализации демонов

Цель: дописать отсутствующие сервисы пайплайна, используя существующие артефакты: генератор событий `event-ingest`, общую папку `/shared`, MySQL шарды и API контракт `pipeline-apid`.

Сервисы для реализации:
- `pipeline-apid` — HTTP API для фиксации движения задач в таблице `pipeline_tracking` (см. `db/ddl.sql`).
- `shard-splitter` — отмечает готовые файлы после `event-ingest`, ставит этап `shard-splitter` в `started/done/failed`, подготавливает задания для воркеров.
- `shard-worker` — обрабатывает задания из очереди `shard-worker`, формирует шард‑специфичные артефакты для отправки в БД, отмечает статусы `started/done/failed`.
- `mysql-sender` — отправляет результаты воркеров в нужный шард MySQL, отмечает `mysql-sender` статусы.

Общие требования
- Все сервисы используют общий том `/shared` и пишут свои артефакты строго в личные поддиректории:
  - `/shared/shard-splitter`
  - `/shared/shard-worker-<shard>`
  - `/shared/mysql-sender`
- При остановке контейнера соответствующая поддиректория очищается (trap на EXIT/TERM/INT уже показан в примерах других Dockerfile).
- Все переходы статусов выполняются через `pipeline-apid` по контракту из `docs/pipeline-apid.md`.
- Статусы строго из множества: `new`, `started`, `done`, `failed`.

---

### 1) pipeline-apid
Назначение: единая точка записи движения задач в таблицу `pipeline_tracking`.

Реализовать HTTP‑эндпоинты (см. деталь в `docs/pipeline-apid.md`):
1. `POST /api/v1/pipeline/files/register` — создать (идемпотентно) записи по всем `shard` для файла `filename`.
2. `POST /api/v1/pipeline/stages/event-ingest/done` — массово проставить `event_ingest_status='done'` по `filename`.
3. `POST /api/v1/pipeline/stages/{stage}/{transition}` — универсальные переходы (`start|done|fail`) для `shard-splitter|shard-worker|mysql-sender`.
4. `POST /api/v1/pipeline/stages/{stage}/retry` — перевести `failed -> new`.
5. `GET /api/v1/pipeline/queues/{stage}` — выдача очереди задач (готовых для этапа), с пагинацией.
6. `GET /api/v1/pipeline/files/{filename}` — статусы по всем шардам файла.
7. `GET /api/v1/pipeline/metrics/summary` — сводка по статусам.

Требования:
- MySQL: использовать переменные окружения (compose уже дает хосты и креды) и схему `db/ddl.sql`.
- Идемпотентность: рекомендовано создать уникальный ключ `(filename, shard)`.
- Обработка ошибок: валидировать входные данные и корректно возвращать коды HTTP.

Критерии приёмки:
- Все эндпоинты работают согласно примерам из `docs/pipeline-apid.md`.
- Регистрация файла и массовая отметка `done` для `event-ingest` отражаются в БД.

---

### 2) shard-splitter
Назначение: прочитать файл `event-ingest` и разложить события по шардам, подготовив входные txt‑файлы для следующего этапа (shard-worker).

Поток:
1. Периодически запрашивать `GET /api/v1/pipeline/queues/shard-splitter` у `pipeline-apid` (берутся записи после `event_ingest_status='done'`).
2. Для каждого `(filename, shard)`: `POST /stages/shard-splitter/start`.
3. Прочитать `/shared/events/<filename>.event-ingest.txt`, отфильтровать события по правилу `shop_id % 2 == shard`.
4. Сформировать txt‑файл для воркера с тем же форматом строк, что у `event-ingest` (одна JSON‑строка на событие). ИМЕННО ТАКОЙ ШАБЛОН ИМЕНИ:
   - `/shared/shard-splitter/<filename>.shard-splitter.<shard>.txt`
   - пример: `20251015172520.shard-splitter.0.txt`, `20251015172520.shard-splitter.1.txt`
5. По завершении: `POST /stages/shard-splitter/done`.
6. При ошибке: `POST /stages/shard-splitter/fail` и лог причины.

Критерии приёмки:
- Этапы переходят через `pipeline-apid`, артефакты создаются в `/shared/shard-splitter`.
- Повторная обработка возможна через `retry`.

---

### 3) shard-worker
Назначение: реализовать бизнес‑логику обработки событий для конкретного шарда, включая дедупликацию, проверку инвариантов и временный кеш для событий, пришедших вне порядка. Результат — подготовленные txt‑файлы для `mysql-sender`.

Поток:
1. Периодически запрашивать `GET /api/v1/pipeline/queues/shard-worker`.
2. Для `(filename, shard)` вызвать `POST /stages/shard-worker/start`.
3. Прочитать входной файл от сплиттера: `/shared/shard-splitter/<filename>.shard-splitter.<shard>.txt`. Формат строк — точно такой же, как у `event-ingest` (одна JSON‑строка на событие).
4. Дедупликация: отбрасывать дубликаты по бизнес‑ключу. Рекомендуемые варианты:
   - для заказных событий (type 1/3): ключ = `order_id` + округлённый `timestamp` (или вся пара `order_id`+`timestamp`),
   - альтернативно — ввести `idempotency_key` в пейлоад и dedupe по нему,
   - хранить ключи в локальном кеше/файле/в памяти в пределах окна обработки батча.
5. Бизнес‑правила (примеры):
   - Нельзя списать деньги, если баланс клиента недостаточен (смоделировать учет баланса в памяти или через предварительно агрегированные операции в батче).
   - Связанные события (например, возврат по заказу до поступления покупки) должны быть выдержаны в локальном кеше ограниченное время (например, до N секунд или до прихода связанного события), с последующей повторной попыткой.
6. Сформировать выходной txt‑файл: `/shared/shard-worker-<shard>/<filename>.shard-worker.<shard>.prepared.txt`. Формат строк — такой же (одна JSON‑строка на событие), но допускается минимальный необходимый набор полей под вставку в `events`: `event_type`, `shop_id`, `user_id` (если есть), `timestamp`, `shard`.
7. По завершении: `POST /stages/shard-worker/done`.
8. При ошибке: `POST /stages/shard-worker/fail`.

Критерии приёмки:
- Для каждой задачи создается выходной txt‑файл в папке воркера.
- Реализованы дедупликация и базовые проверки инвариантов.
- Поддержан временной кеш для out‑of‑order событий (с параметром задержки).
- Статусы этапов отражаются в `pipeline_tracking`.

---

### 4) mysql-sender
Назначение: забрать подготовленные воркером события и вставить их в нужный шард MySQL (таблица `events`).

Поток:
1. Периодически запрашивать `GET /api/v1/pipeline/queues/mysql-sender`.
2. Для `(filename, shard)` вызвать `POST /stages/mysql-sender/start`.
3. Прочитать `/shared/shard-worker-<shard>/<filename>.shard-worker.<shard>.prepared.txt`. Формат строк — одна JSON‑строка на событие, совместимый с предыдущими этапами.
4. Вставить строки в `events` соответствующего шарда:
   - `id` — автоинкремент,
   - `event_type`, `shop_id`, `user_id`, `timestamp`, `shard`.
   - Индексы уже заданы в `ddl.sql`.
5. По завершении: `POST /stages/mysql-sender/done`.
6. При ошибке: `POST /stages/mysql-sender/fail`.

Критерии приёмки:
- Данные действительно появляются в таблице `events` выбранного шарда.
- Корректно проставляются статусы в `pipeline_tracking`.

---

### Набор инструментов и подсказки
- Брать примеры SQL и переходов статусов из `db/pipeline-examples.sql` и `docs/pipeline-apid.md`.
- Временной формат: `YYYY-MM-DD HH:MM:SS.MS` (UTC).
- Очереди — через `GET /queues/{stage}`; для конкурентной выборки продумать блокировки/атомарные апдейты.
- Логи — простой stdout; в `/shared/<service>/activity.log` можно писать служебные сообщения.
