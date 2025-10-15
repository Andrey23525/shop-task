# Makefile для управления проектом

.PHONY: help build up down logs test load-test clean

# Переменные
DOCKER_COMPOSE = docker-compose
PROJECT_NAME = shop-task

help: ## Показать справку
	@echo "Доступные команды:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Собрать все сервисы
	$(DOCKER_COMPOSE) build

up: ## Запустить все сервисы
	$(DOCKER_COMPOSE) up -d

down: ## Остановить все сервисы
	$(DOCKER_COMPOSE) down

logs: ## Показать логи всех сервисов
	$(DOCKER_COMPOSE) logs -f

logs-ingest: ## Показать логи event-ingest
	$(DOCKER_COMPOSE) logs -f event-ingest

logs-splitter: ## Показать логи shard-splitter
	$(DOCKER_COMPOSE) logs -f shard-splitter

logs-worker-0: ## Показать логи shard-worker-0
	$(DOCKER_COMPOSE) logs -f shard-worker-0

logs-worker-1: ## Показать логи shard-worker-1
	$(DOCKER_COMPOSE) logs -f shard-worker-1

logs-balance: ## Показать логи balance-daemon
	$(DOCKER_COMPOSE) logs -f balance-daemon

logs-cache: ## Показать логи cache-service
	$(DOCKER_COMPOSE) logs -f cache-service

test: ## Запустить все тесты
	$(DOCKER_COMPOSE) up -d
	sleep 30
	$(DOCKER_COMPOSE) run --rm test-runner

test-unit: ## Запустить только unit тесты
	$(DOCKER_COMPOSE) run --rm test-runner pytest unit/ -v

test-integration: ## Запустить только интеграционные тесты
	$(DOCKER_COMPOSE) run --rm test-runner pytest integration/ -v

load-test: ## Запустить нагрузочные тесты
	$(DOCKER_COMPOSE) run --rm test-runner pytest load/ -v

clean: ## Очистить все контейнеры и volumes
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker system prune -f

restart: ## Перезапустить все сервисы
	$(DOCKER_COMPOSE) restart

status: ## Показать статус сервисов
	$(DOCKER_COMPOSE) ps

shell-ingest: ## Подключиться к контейнеру event-ingest
	$(DOCKER_COMPOSE) exec event-ingest sh

shell-mysql-0: ## Подключиться к MySQL шарда 0
	$(DOCKER_COMPOSE) exec mysql-shard-0 mysql -u shop_user -pshop_password shop_shard_0

shell-mysql-1: ## Подключиться к MySQL шарда 1
	$(DOCKER_COMPOSE) exec mysql-shard-1 mysql -u shop_user -pshop_password shop_shard_1

shell-redis: ## Подключиться к Redis
	$(DOCKER_COMPOSE) exec redis redis-cli

event-stats: ## Показать статистику событий
	curl http://localhost:8080/api/v1/events/stats

list-binlogs: ## Показать список текстовых логов
	python3 scripts/read-binlog.py --list ./shared/events

read-latest-binlog: ## Прочитать последний текстовый лог
	python3 scripts/read-binlog.py --latest ./shared/events

read-binlog: ## Прочитать указанный текстовый лог (использование: make read-binlog FILE=20250101000000.event-ingest.txt)
	python3 scripts/read-binlog.py ./shared/events/$(FILE)

init-db: ## Инициализировать базу данных
	$(DOCKER_COMPOSE) up -d mysql-shard-0 mysql-shard-1 redis
	sleep 10
	@echo "База данных инициализирована"

dev: ## Запустить в режиме разработки
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml up -d

# Команды для разработки
dev-build: ## Собрать сервисы для разработки
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml build

dev-logs: ## Показать логи в режиме разработки
	$(DOCKER_COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml logs -f

# Команды для отдельных сервисов
up-ingest: ## Запустить только event-ingest
	$(DOCKER_COMPOSE) -f docker-compose/event-ingest.yml up -d

up-mysql: ## Запустить только MySQL
	$(DOCKER_COMPOSE) -f docker-compose/mysql.yml up -d

up-redis: ## Запустить только Redis
	$(DOCKER_COMPOSE) -f docker-compose/redis.yml up -d

up-workers: ## Запустить только shard-workers
	$(DOCKER_COMPOSE) -f docker-compose/shard-workers.yml up -d

up-splitter: ## Запустить только shard-splitter
	$(DOCKER_COMPOSE) -f docker-compose/shard-splitter.yml up -d

up-balance: ## Запустить только balance-daemon
	$(DOCKER_COMPOSE) -f docker-compose/balance-daemon.yml up -d

up-cache: ## Запустить только cache-service
	$(DOCKER_COMPOSE) -f docker-compose/cache-service.yml up -d
