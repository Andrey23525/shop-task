package models

import (
	"encoding/json"
	"time"
)

// Event представляет событие в системе
type Event struct {
	Timestamp string         `json:"ts"`
	EventType int            `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

// EventBatch представляет батч событий для обработки
type EventBatch struct {
	Events []Event `json:"events"`
}

// EventResponse представляет ответ на запрос событий
type EventResponse struct {
	RejectedEvents   []RejectedEvent `json:"rejected_events,omitempty"`
}

// RejectedEvent представляет отклоненное событие
type RejectedEvent struct {
	Reason  string `json:"reason"`
}

// EventType 0: Изменение количества
type RestockEvent struct {
	ShopID      int64  `json:"shop_id"`
	GoodID      int64  `json:"good_id"`
	DeltaCount  int    `json:"delta_count"`
	Reason      string `json:"reason"`
	SubEventType string `json:"sub_event_type"` // restock/defect/loss
}

// EventType 1: Покупка/продажа
type PurchaseEvent struct {
	OrderID       int64   `json:"order_id"`
	UserID        int64   `json:"user_id"`
	ShopID        int64   `json:"shop_id"`
	GoodID        int64   `json:"good_id"`
	Qty           int     `json:"qty"`
	PriceAtOrder  *float64 `json:"price_at_order,omitempty"`
}

// EventType 2: Изменение цены
type PriceChangeEvent struct {
	ShopID    int64   `json:"shop_id"`
	GoodID    int64   `json:"good_id"`
	NewPrice  float64 `json:"new_price"`
	OldPrice  *float64 `json:"old_price,omitempty"`
}

// EventType 3: Возврат
type ReturnEvent struct {
	OrderID       int64    `json:"order_id"`
	UserID        int64    `json:"user_id"`
	GoodID        int64    `json:"good_id"`
	Qty           int      `json:"qty"`
	RefundAmount  *float64 `json:"refund_amount,omitempty"`
}

// EventType 4: Открытие/закрытие магазина
type ShopStatusEvent struct {
	ShopID int64 `json:"shop_id"`
	Active bool  `json:"active"`
}

// NewEvent создает новое событие
func NewEvent(eventType int, payload json.RawMessage) Event {
	return Event{
		Timestamp: time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		EventType: eventType,
		Payload:   payload,
	}
}
