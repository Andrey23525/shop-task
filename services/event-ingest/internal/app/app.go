package app

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shop-event-ingest/internal/config"
	"shop-event-ingest/internal/handlers"
	"shop-event-ingest/internal/services"

	"github.com/gin-gonic/gin"
)

type App struct {
	config     *config.Config
	server     *http.Server
	services   *services.Services
}

func New(cfg *config.Config) *App {
	// Инициализируем сервисы
	svc := services.New(cfg)

	// Инициализируем обработчики
	handlers := handlers.New(svc)

	// Настраиваем роутер
	router := setupRouter(handlers)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	app := &App{
		config:   cfg,
		server:   server,
		services: svc,
	}

	// Запускаем автоматическую генерацию событий при старте
	go app.startEventGeneration()

	return app
}

func (a *App) Run() error {
	// Запускаем сервер в горутине
	go func() {
		log.Printf("Starting server on %s", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Ждем сигнал для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	log.Println("Server exited")
	return nil
}

// startEventGeneration запуск автоматической генерации событий
func (a *App) startEventGeneration() {
	// Получаем интервал из конфигурации
	interval := time.Duration(a.config.Events.GenerationInterval) * time.Second
	cfgMax := a.config.Events.EventsPerBatch
	if a.config.Events.MaxBatchSize > 0 && a.config.Events.MaxBatchSize < cfgMax {
		cfgMax = a.config.Events.MaxBatchSize
	}
	if cfgMax <= 0 {
		cfgMax = 1
	}

	// Ждем до следующей границы интервала
	now := time.Now()
	nextBoundary := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 
		((now.Second()/int(interval.Seconds()))+1)*int(interval.Seconds()), 0, now.Location())
	if nextBoundary.After(now.Add(interval)) {
		nextBoundary = nextBoundary.Add(-interval)
	}
	
	waitTime := nextBoundary.Sub(now)
	log.Printf("Waiting %v until next %v boundary...", waitTime, interval)
	time.Sleep(waitTime)
	
	log.Printf("Starting automatic event generation (interval: %v, max events: %d)...", interval, cfgMax)
	
	// Запускаем генерацию событий по интервалу из конфигурации
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Генерируем случайное количество событий (1..cfgMax)
			count := 1 + rand.Intn(cfgMax)
			log.Printf("Generating %d events...", count)
			if err := a.services.GenerateTestEvents(count); err != nil {
				log.Printf("Failed to generate events: %v", err)
			} else {
				log.Printf("Successfully generated %d events", count)
			}
		}
	}
}

func setupRouter(handlers *handlers.Handlers) *gin.Engine {
	// Настраиваем Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Health check endpoints
	router.GET("/health", handlers.Health)
	router.GET("/ready", handlers.Ready)

	// API endpoints
	api := router.Group("/api/v1")
	{
		api.GET("/events/stats", handlers.GetEventStats)
	}

	return router
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
