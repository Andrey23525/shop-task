package main

import (
	"log"
	"shop-event-ingest/internal/app"
	"shop-event-ingest/internal/config"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Создаем и запускаем приложение
	application := app.New(cfg)
	if err := application.Run(); err != nil {
		log.Fatalf("Failed to run application: %v", err)
	}
}
