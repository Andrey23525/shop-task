package handlers

import (
	"encoding/json"
	"net/http"
	"shop-event-ingest/internal/models"
	"shop-event-ingest/internal/services"
	"time"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	services *services.Services
}

func New(services *services.Services) *Handlers {
	return &Handlers{
		services: services,
	}
}

// Health проверка здоровья сервиса
func (h *Handlers) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"service":   "event-ingest",
	})
}

// Ready проверка готовности сервиса
func (h *Handlers) Ready(c *gin.Context) {
	// Проверяем подключения к БД и Redis
	if err := h.services.CheckHealth(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	})
}

// StartGeneration запуск генерации событий
func (h *Handlers) StartGeneration(c *gin.Context) {
	var request struct {
		IntervalSeconds int `json:"interval_seconds" binding:"required,min=1,max=3600"`
		EventTypes      []int `json:"event_types,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	if err := h.services.StartEventGeneration(request.IntervalSeconds, request.EventTypes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to start event generation",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Event generation started",
		"interval_seconds": request.IntervalSeconds,
		"event_types": request.EventTypes,
	})
}

// StopGeneration остановка генерации событий
func (h *Handlers) StopGeneration(c *gin.Context) {
	if err := h.services.StopEventGeneration(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to stop event generation",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Event generation stopped",
	})
}


// GenerateEvents генерация тестовых событий
func (h *Handlers) GenerateEvents(c *gin.Context) {
	var request struct {
		Count int `json:"count" binding:"required,min=1,max=1000"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	if err := h.services.GenerateTestEvents(request.Count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate events",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Events generated successfully",
		"count": request.Count,
	})
}

// GetEventStats получение статистики событий
func (h *Handlers) GetEventStats(c *gin.Context) {
	stats, err := h.services.GetEventStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get event stats",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// validateEvent валидация события
func (h *Handlers) validateEvent(event models.Event) error {
	// Проверяем обязательные поля
	if event.EventType < 0 || event.EventType > 4 {
		return &ValidationError{Field: "event_type", Message: "Event type must be between 0 and 4"}
	}

	// Валидируем payload в зависимости от типа события
	switch event.EventType {
	case 0: // Restock
		return h.validateRestockEvent(event.Payload)
	case 1: // Purchase
		return h.validatePurchaseEvent(event.Payload)
	case 2: // Price Change
		return h.validatePriceChangeEvent(event.Payload)
	case 3: // Return
		return h.validateReturnEvent(event.Payload)
	case 4: // Shop Status
		return h.validateShopStatusEvent(event.Payload)
	default:
		return &ValidationError{Field: "event_type", Message: "Unknown event type"}
	}
}

// validateRestockEvent валидация события пополнения остатков
func (h *Handlers) validateRestockEvent(payload json.RawMessage) error {
	var event models.RestockEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return &ValidationError{Field: "payload", Message: "Invalid restock event payload: " + err.Error()}
	}

	if event.ShopID <= 0 {
		return &ValidationError{Field: "shop_id", Message: "Shop ID must be positive"}
	}

	if event.GoodID <= 0 {
		return &ValidationError{Field: "good_id", Message: "Good ID must be positive"}
	}

	if event.DeltaCount == 0 {
		return &ValidationError{Field: "delta_count", Message: "Delta count cannot be zero"}
	}

	if event.SubEventType == "" {
		return &ValidationError{Field: "sub_event_type", Message: "Sub event type is required"}
	}

	validSubTypes := map[string]bool{
		"restock": true,
		"defect":  true,
		"loss":    true,
	}

	if !validSubTypes[event.SubEventType] {
		return &ValidationError{Field: "sub_event_type", Message: "Invalid sub event type"}
	}

	return nil
}

// validatePurchaseEvent валидация события покупки
func (h *Handlers) validatePurchaseEvent(payload json.RawMessage) error {
	var event models.PurchaseEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return &ValidationError{Field: "payload", Message: "Invalid purchase event payload: " + err.Error()}
	}

	if event.OrderID <= 0 {
		return &ValidationError{Field: "order_id", Message: "Order ID must be positive"}
	}

	if event.UserID <= 0 {
		return &ValidationError{Field: "user_id", Message: "User ID must be positive"}
	}

	if event.ShopID <= 0 {
		return &ValidationError{Field: "shop_id", Message: "Shop ID must be positive"}
	}

	if event.GoodID <= 0 {
		return &ValidationError{Field: "good_id", Message: "Good ID must be positive"}
	}

	if event.Qty <= 0 {
		return &ValidationError{Field: "qty", Message: "Quantity must be positive"}
	}

	return nil
}

// validatePriceChangeEvent валидация события изменения цены
func (h *Handlers) validatePriceChangeEvent(payload json.RawMessage) error {
	var event models.PriceChangeEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return &ValidationError{Field: "payload", Message: "Invalid price change event payload: " + err.Error()}
	}

	if event.ShopID <= 0 {
		return &ValidationError{Field: "shop_id", Message: "Shop ID must be positive"}
	}

	if event.GoodID <= 0 {
		return &ValidationError{Field: "good_id", Message: "Good ID must be positive"}
	}

	if event.NewPrice <= 0 {
		return &ValidationError{Field: "new_price", Message: "New price must be positive"}
	}

	return nil
}

// validateReturnEvent валидация события возврата
func (h *Handlers) validateReturnEvent(payload json.RawMessage) error {
	var event models.ReturnEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return &ValidationError{Field: "payload", Message: "Invalid return event payload: " + err.Error()}
	}

	if event.OrderID <= 0 {
		return &ValidationError{Field: "order_id", Message: "Order ID must be positive"}
	}

	if event.UserID <= 0 {
		return &ValidationError{Field: "user_id", Message: "User ID must be positive"}
	}

	if event.GoodID <= 0 {
		return &ValidationError{Field: "good_id", Message: "Good ID must be positive"}
	}

	if event.Qty <= 0 {
		return &ValidationError{Field: "qty", Message: "Quantity must be positive"}
	}

	return nil
}

// validateShopStatusEvent валидация события изменения статуса магазина
func (h *Handlers) validateShopStatusEvent(payload json.RawMessage) error {
	var event models.ShopStatusEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return &ValidationError{Field: "payload", Message: "Invalid shop status event payload: " + err.Error()}
	}

	if event.ShopID <= 0 {
		return &ValidationError{Field: "shop_id", Message: "Shop ID must be positive"}
	}

	return nil
}

// ValidationError ошибка валидации
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
