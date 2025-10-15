package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"shop-event-ingest/internal/config"
	"shop-event-ingest/internal/models"
	"sync"
	"time"
)

type Services struct {
	config     *config.Config
	eventsDir  string
	
	// Генерация событий
	generationCtx    context.Context
	generationCancel context.CancelFunc
	generationMutex  sync.RWMutex
	isGenerating     bool
	httpClient       *http.Client
}

func New(cfg *config.Config) *Services {
	// Создаем папку для сохранения событий
	eventsDir := "/shared/events"
	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		// Fallback на локальную папку
		eventsDir = "./events"
		os.MkdirAll(eventsDir, 0755)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Services{
		config:           cfg,
		eventsDir:        eventsDir,
		generationCtx:    ctx,
		generationCancel: cancel,
		isGenerating:     false,
		httpClient:       &http.Client{Timeout: 5 * time.Second},
	}
}

// SaveEvent сохранение события
func (s *Services) SaveEvent(event models.Event) error {
	// Сохраняем в файл (текстовый лог)
	if err := s.saveEventToFile(event); err != nil {
		return fmt.Errorf("failed to save event to file: %w", err)
	}

	return nil
}

// GetEvent получение события по ID (не поддерживается без БД)
func (s *Services) GetEvent(eventID string) (*models.Event, error) {
	return nil, fmt.Errorf("get by id not supported in file-only mode: %s", eventID)
}

// CheckHealth проверка здоровья сервиса (нет внешних зависимостей)
func (s *Services) CheckHealth() error {
	return nil
}

// saveEventToFile сохранение события в текстовый файл
func (s *Services) saveEventToFile(event models.Event) error {
	// Создаем папку для логов, если не существует
	if err := os.MkdirAll(s.eventsDir, 0755); err != nil {
		return fmt.Errorf("failed to create events directory: %w", err)
	}

	// Создаем файл с именем по формату YYYYMMDDHHMMSS.event-ingest.txt
	now := time.Now()
	filename := filepath.Join(s.eventsDir, fmt.Sprintf("%s.event-ingest.txt", now.Format("20060102150405")))

	// Подготавливаем данные для записи
	eventData := map[string]interface{}{
		"timestamp":  event.Timestamp,
		"event_type": event.EventType,
		"payload":    event.Payload,
		"saved_at":   now.UTC().Format("2006-01-02 15:04:05.000"),
	}

	// Конвертируем в JSON
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Открываем файл для записи (append mode)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Записываем событие как текстовую строку (1 событие на строку)
	if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
		return fmt.Errorf("failed to write event data: %w", err)
	}

	return nil
}

// GenerateTestEvents генерация тестовых событий
func (s *Services) GenerateTestEvents(count int) error {
	// Создаем папку для логов, если не существует
	if err := os.MkdirAll(s.eventsDir, 0755); err != nil {
		return fmt.Errorf("failed to create events directory: %w", err)
	}

	// Формируем префикс файла и полный путь
	now := time.Now()
	prefix := now.Format("20060102150405")
	filename := filepath.Join(s.eventsDir, fmt.Sprintf("%s.event-ingest.txt", prefix))

	// Регистрируем файл в pipeline-api
	s.registerPipelineFile(prefix)

	// Открываем файл для записи (append mode)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	for i := 0; i < count; i++ {
		// Генерируем случайный тип события
		eventType := i % 5

		var payload json.RawMessage
		var err error

		switch eventType {
		case 0: // Restock
			restockEvent := models.RestockEvent{
				ShopID:       int64(2 + (i % 10)),
				GoodID:       int64(1 + (i % 5)),
				DeltaCount:   1 + (i % 10),
				Reason:       "test_generation",
				SubEventType: "restock",
			}
			payload, err = json.Marshal(restockEvent)

		case 1: // Purchase
			orderID := rand.Int63()
			purchaseEvent := models.PurchaseEvent{
				OrderID:      orderID,
				UserID:       1 + int64(i%4),
				ShopID:       int64(2 + (i % 10)),
				GoodID:       int64(1 + (i % 5)),
				Qty:          1 + (i % 3),
				PriceAtOrder: &[]float64{99.99, 199.99, 299.99}[i%3],
			}
			payload, err = json.Marshal(purchaseEvent)

		case 2: // Price Change
			priceEvent := models.PriceChangeEvent{
				ShopID:   int64(2 + (i % 10)),
				GoodID:   int64(1 + (i % 5)),
				NewPrice: 99.99 + float64(i%100),
			}
			payload, err = json.Marshal(priceEvent)

		case 3: // Return
			orderID := rand.Int63()
			returnEvent := models.ReturnEvent{
				OrderID:      orderID,
				UserID:       1 + int64(i%4),
				GoodID:       int64(1 + (i % 5)),
				Qty:          1 + (i % 3),
				RefundAmount: &[]float64{99.99, 199.99, 299.99}[i%3],
			}
			payload, err = json.Marshal(returnEvent)

		case 4: // Shop Status
			statusEvent := models.ShopStatusEvent{
				ShopID: int64(2 + (i % 10)),
				Active: i%2 == 0,
			}
			payload, err = json.Marshal(statusEvent)
		}

		if err != nil {
			return fmt.Errorf("failed to marshal event %d: %w", i, err)
		}

		// Подготавливаем данные для записи
		eventData := map[string]interface{}{
			"timestamp":  time.Now().UTC().Format("2006-01-02 15:04:05.000"),
			"event_type": eventType,
			"payload":    payload,
			"saved_at":   now.UTC().Format("2006-01-02 15:04:05.000"),
		}

		// Конвертируем в JSON
		jsonData, err := json.Marshal(eventData)
		if err != nil {
			return fmt.Errorf("failed to marshal event data: %w", err)
		}

		// Записываем событие как текстовую строку (1 событие на строку)
		if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
			return fmt.Errorf("failed to write event data: %w", err)
		}
	}

	// Отмечаем завершение генерации для всех шардов
	s.markEventIngestDone(prefix)

	return nil
}

// GetEventStats получение статистики событий
func (s *Services) GetEventStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Проверяем папку с логами
	if _, err := os.Stat(s.eventsDir); os.IsNotExist(err) {
		stats["total_files"] = 0
		stats["total_events"] = 0
		stats["events_directory"] = s.eventsDir
		return stats, nil
	}

	files, err := os.ReadDir(s.eventsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read events directory: %w", err)
	}

	totalFiles := 0
	totalEvents := 0
	fileStats := make(map[string]interface{})

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".binlog" {
			totalFiles++
			
			filePath := filepath.Join(s.eventsDir, file.Name())
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				continue
			}

			// Подсчитываем события в бинарном файле
			eventCount, err := s.countEventsInBinlog(filePath)
			if err != nil {
				continue
			}

			totalEvents += eventCount
			fileStats[file.Name()] = map[string]interface{}{
				"size":        fileInfo.Size(),
				"events":      eventCount,
				"modified_at": fileInfo.ModTime(),
			}
		}
	}

	stats["total_files"] = totalFiles
	stats["total_events"] = totalEvents
	stats["events_directory"] = s.eventsDir
	stats["files"] = fileStats

	return stats, nil
}

// countEventsInBinlog подсчет событий в бинарном файле
func (s *Services) countEventsInBinlog(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	eventCount := 0
	
	for {
		// Читаем длину данных (4 байта)
		lengthBytes := make([]byte, 4)
		if _, err := file.Read(lengthBytes); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return eventCount, err
		}

		// Конвертируем длину из big-endian
		length := int(lengthBytes[0])<<24 | int(lengthBytes[1])<<16 | int(lengthBytes[2])<<8 | int(lengthBytes[3])
		
		// Читаем данные события
		eventData := make([]byte, length)
		if _, err := file.Read(eventData); err != nil {
			return eventCount, err
		}

		eventCount++
	}

	return eventCount, nil
}

// StartEventGeneration запуск автоматической генерации событий
func (s *Services) StartEventGeneration(intervalSeconds int, eventTypes []int) error {
	s.generationMutex.Lock()
	defer s.generationMutex.Unlock()

	if s.isGenerating {
		return fmt.Errorf("event generation is already running")
	}

	// Если eventTypes пустой, используем все типы
	if len(eventTypes) == 0 {
		eventTypes = []int{0, 1, 2, 3, 4}
	}

	s.isGenerating = true

	// Запускаем генерацию в отдельной горутине
	go s.runEventGeneration(intervalSeconds, eventTypes)

	return nil
}

// StopEventGeneration остановка генерации событий
func (s *Services) StopEventGeneration() error {
	s.generationMutex.Lock()
	defer s.generationMutex.Unlock()

	if !s.isGenerating {
		return fmt.Errorf("event generation is not running")
	}

	s.generationCancel()
	s.isGenerating = false

	// Создаем новый контекст для следующего запуска
	s.generationCtx, s.generationCancel = context.WithCancel(context.Background())

	return nil
}

// runEventGeneration основная логика генерации событий
func (s *Services) runEventGeneration(intervalSeconds int, eventTypes []int) {
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.generationCtx.Done():
			return
		case <-ticker.C:
			// Генерируем случайное событие
			event := s.generateRandomEvent(eventTypes)
			if err := s.SaveEvent(event); err != nil {
				fmt.Printf("Failed to save generated event: %v\n", err)
			} else {
				fmt.Printf("Generated event (type %d)\n", event.EventType)
			}
		}
	}
}

// generateRandomEvent генерация случайного события
func (s *Services) generateRandomEvent(eventTypes []int) models.Event {
	// Выбираем случайный тип события
	eventType := eventTypes[rand.Intn(len(eventTypes))]
	
	var payload json.RawMessage
	var err error

	switch eventType {
	case 0: // Restock
		restockEvent := models.RestockEvent{
			ShopID:       int64(1 + rand.Intn(20)), // shop_id от 1 до 20
			GoodID:       int64(1 + rand.Intn(10)), // good_id от 1 до 10
			DeltaCount:   rand.Intn(20) - 10,       // от -10 до +10
			Reason:       s.getRandomReason(),
			SubEventType: s.getRandomSubEventType(),
		}
		payload, err = json.Marshal(restockEvent)

	case 1: // Purchase
		price := 50.0 + rand.Float64()*950.0 // от 50 до 1000
		purchaseEvent := models.PurchaseEvent{
			OrderID:      rand.Int63(),
			UserID:       int64(1 + rand.Intn(10)), // user_id от 1 до 10
			ShopID:       int64(1 + rand.Intn(20)), // shop_id от 1 до 20
			GoodID:       int64(1 + rand.Intn(10)), // good_id от 1 до 10
			Qty:          1 + rand.Intn(5),          // от 1 до 5
			PriceAtOrder: &price,
		}
		payload, err = json.Marshal(purchaseEvent)

	case 2: // Price Change
		oldPrice := 50.0 + rand.Float64()*950.0
		newPrice := oldPrice + (rand.Float64()-0.5)*200.0 // изменение от -100 до +100
		if newPrice < 1.0 {
			newPrice = 1.0
		}
		priceEvent := models.PriceChangeEvent{
			ShopID:   int64(1 + rand.Intn(20)), // shop_id от 1 до 20
			GoodID:   int64(1 + rand.Intn(10)), // good_id от 1 до 10
			NewPrice: newPrice,
			OldPrice: &oldPrice,
		}
		payload, err = json.Marshal(priceEvent)

	case 3: // Return
		refundAmount := 50.0 + rand.Float64()*950.0
		returnEvent := models.ReturnEvent{
			OrderID:      rand.Int63(),
			UserID:       int64(1 + rand.Intn(10)), // user_id от 1 до 10
			GoodID:       int64(1 + rand.Intn(10)), // good_id от 1 до 10
			Qty:          1 + rand.Intn(3),          // от 1 до 3
			RefundAmount: &refundAmount,
		}
		payload, err = json.Marshal(returnEvent)

	case 4: // Shop Status
		statusEvent := models.ShopStatusEvent{
			ShopID: int64(1 + rand.Intn(20)), // shop_id от 1 до 20
			Active: rand.Intn(2) == 1,        // случайно true/false
		}
		payload, err = json.Marshal(statusEvent)
	}

	if err != nil {
		// Fallback на простое событие
		payload = []byte(`{"error": "failed to generate payload"}`)
	}

	return models.NewEvent(eventType, payload)
}

// getRandomReason возвращает случайную причину
func (s *Services) getRandomReason() string {
	reasons := []string{
		"restock", "defect", "loss", "inventory_adjustment", 
		"damaged_goods", "expired_items", "theft", "return_to_supplier",
	}
	return reasons[rand.Intn(len(reasons))]
}

// getRandomSubEventType возвращает случайный подтип события
func (s *Services) getRandomSubEventType() string {
	subTypes := []string{"restock", "defect", "loss"}
	return subTypes[rand.Intn(len(subTypes))]
}

// IsGenerating проверка, генерируются ли события
func (s *Services) IsGenerating() bool {
	s.generationMutex.RLock()
	defer s.generationMutex.RUnlock()
	return s.isGenerating
}

// registerPipelineFile notifies pipeline-api to register file per shards
func (s *Services) registerPipelineFile(prefix string) {
	if s.config.Pipeline.URL == "" || s.config.Pipeline.ShardsCount <= 0 {
		return
	}
	body := map[string]interface{}{
		"filename": prefix,
		"shards":   make([]int, 0, s.config.Pipeline.ShardsCount),
	}
	for i := 0; i < s.config.Pipeline.ShardsCount; i++ {
		body["shards"] = append(body["shards"].([]int), i)
	}
	b, _ := json.Marshal(body)
	_, _ = s.httpClient.Post(s.config.Pipeline.URL+"/files/register", "application/json", bytes.NewReader(b))
}

// markEventIngestDone marks event-ingest done for all shards of the file
func (s *Services) markEventIngestDone(prefix string) {
	if s.config.Pipeline.URL == "" {
		return
	}
	body := map[string]interface{}{"filename": prefix}
	b, _ := json.Marshal(body)
	_, _ = s.httpClient.Post(s.config.Pipeline.URL+"/stages/event-ingest/done", "application/json", bytes.NewReader(b))
}
