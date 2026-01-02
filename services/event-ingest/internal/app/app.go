package app

import (
	"context"
	"database/sql"
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
	_ "github.com/go-sql-driver/mysql"
)

type App struct {
	config   *config.Config
	server   *http.Server
	services *services.Services
	db       *sql.DB
}

// Структуры для pipeline endpoints
type IngestRequest struct {
	Filename string  `json:"filename" binding:"required"`
	Shards   []int32 `json:"shards" binding:"required"`
}

type IngestResponse struct {
	Filename string  `json:"filename"`
	Shards   []int32 `json:"shards"`
	Created  bool    `json:"created"`
}

type IngestDoneRequest struct {
	Filename string `json:"filename" binding:"required"`
}

type IngestDoneResponse struct {
	Filename    string `json:"filename"`
	UpdatedRows int64  `json:"updated_rows"`
}

func New(cfg *config.Config) *App {
	// Инициализируем сервисы
	svc := services.New(cfg)

	// Подключаемся к БД для pipeline tracking
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "shop_user:shop_password@tcp(mysql-pipeline:3306)/pipeline_db?parseTime=true"
	}

	db, err := sql.Open("mysql", databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Проверяем подключение
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Настраиваем пул соединений
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)

	// Создаем таблицу если её нет
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS pipeline_tracking (
			id INT AUTO_INCREMENT PRIMARY KEY,
			filename VARCHAR(255) NOT NULL,
			shard INT NOT NULL,
			event_ingest_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
			transform_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
			load_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY unique_filename_shard (filename, shard)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	log.Println("Database connection established and table created")

	// Инициализируем обработчики
	hdl := handlers.New(svc)

	// Настраиваем роутер
	router := setupRouter(hdl, db)

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
		db:       db,
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

	// Закрываем БД
	if a.db != nil {
		a.db.Close()
	}

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

func setupRouter(handlers *handlers.Handlers, db *sql.DB) *gin.Engine {
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
	api := router.Group("/api")
	{
		api.GET("/events/stats", handlers.GetEventStats)
	}

	// Pipeline endpoints
	pipeline := router.Group("/pipeline")
	{
		pipeline.POST("/files/register", filesRegisterHandler(db))
		pipeline.POST("/stages/event-ingest/done", stagesEventIngestDoneHandler(db))
	}

	return router
}

// filesRegisterHandler - обработчик регистрации файлов
func filesRegisterHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req IngestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		log.Printf("Register request: %+v", req)

		anyCreated := false

		for _, shard := range req.Shards {
			query := `
				INSERT INTO pipeline_tracking (
					filename,
					shard,
					event_ingest_status,
					transform_status,
					load_status
				)
				VALUES (?, ?, 'started', 'new', 'new')
				ON DUPLICATE KEY UPDATE filename = filename
			`

			result, err := db.Exec(query, req.Filename, shard)
			if err != nil {
				log.Printf("Database error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}

			// RowsAffected() вернёт 1 если запись вставлена, 2 если была обновлена
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				log.Printf("Error getting rows affected: %v", err)
				continue
			}

			if rowsAffected == 1 {
				anyCreated = true
			}
		}

		response := IngestResponse{
			Filename: req.Filename,
			Shards:   req.Shards,
			Created:  anyCreated,
		}

		c.JSON(http.StatusOK, response)
	}
}

// stagesEventIngestDoneHandler - обработчик завершения ingest
func stagesEventIngestDoneHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req IngestDoneRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		log.Printf("Ingest done request: %+v", req)

		query := `
			UPDATE pipeline_tracking
			SET event_ingest_status = 'done', updated_at = NOW()
			WHERE filename = ?
		`

		result, err := db.Exec(query, req.Filename)
		if err != nil {
			log.Printf("Database error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("Error getting rows affected: %v", err)
			rowsAffected = 0
		}

		response := IngestDoneResponse{
			Filename:    req.Filename,
			UpdatedRows: rowsAffected,
		}

		c.JSON(http.StatusOK, response)
	}
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