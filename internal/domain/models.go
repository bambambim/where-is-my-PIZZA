package domain

import (
	"database/sql"
	"time"
)

//=================================================
// API Request/Response Models
//=================================================

// CreateOrderItemRequest represents a single item in a new order request.
type CreateOrderItemRequest struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// CreateOrderRequest is the structure for the POST /orders API request body.
// Pointers are used for optional fields to distinguish between a zero-value and a field that was not provided.
type CreateOrderRequest struct {
	CustomerName    string                   `json:"customer_name"`
	OrderType       string                   `json:"order_type"`
	TableNumber     *int                     `json:"table_number,omitempty"`
	DeliveryAddress *string                  `json:"delivery_address,omitempty"`
	Items           []CreateOrderItemRequest `json:"items"`
}

// CreateOrderResponse is the structure for the POST /orders API response body.
type CreateOrderResponse struct {
	OrderNumber string  `json:"order_number"`
	Status      string  `json:"status"`
	TotalAmount float64 `json:"total_amount"`
}

//=================================================
// Database Table Models
//=================================================

// Order maps to the "orders" table in the database.
type Order struct {
	ID              int64          `db:"id"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	Number          string         `db:"number"`
	CustomerName    string         `db:"customer_name"`
	Type            string         `db:"type"`
	TableNumber     sql.NullInt32  `db:"table_number"`
	DeliveryAddress sql.NullString `db:"delivery_address"`
	TotalAmount     float64        `db:"total_amount"`
	Priority        int            `db:"priority"`
	Status          string         `db:"status"`
	ProcessedBy     sql.NullString `db:"processed_by"`
	CompletedAt     sql.NullTime   `db:"completed_at"`
}

// OrderItem maps to the "order_items" table in the database.
type OrderItem struct {
	ID        int64     `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	OrderID   int64     `db:"order_id"`
	Name      string    `db:"name"`
	Quantity  int       `db:"quantity"`
	Price     float64   `db:"price"`
}

// OrderStatusLog maps to the "order_status_log" table in the database.
type OrderStatusLog struct {
	ID        int64          `db:"id"`
	CreatedAt time.Time      `db:"created_at"`
	OrderID   int64          `db:"order_id"`
	Status    string         `db:"status"`
	ChangedBy sql.NullString `db:"changed_by"`
	ChangedAt time.Time      `db:"changed_at"`
	Notes     sql.NullString `db:"notes"`
}

// Worker maps to the "workers" table in the database.
type Worker struct {
	ID              int64     `db:"id"`
	CreatedAt       time.Time `db:"created_at"`
	Name            string    `db:"name"`
	Type            string    `db:"type"`
	Status          string    `db:"status"`
	LastSeen        time.Time `db:"last_seen"`
	OrdersProcessed int       `db:"orders_processed"`
}

//=================================================
// RabbitMQ Message Models
//=================================================

// OrderMessage is the message published by the Order Service to the orders_topic.
type OrderMessage struct {
	OrderNumber     string                   `json:"order_number"`
	CustomerName    string                   `json:"customer_name"`
	OrderType       string                   `json:"order_type"`
	TableNumber     *int                     `json:"table_number,omitempty"`
	DeliveryAddress *string                  `json:"delivery_address,omitempty"`
	Items           []CreateOrderItemRequest `json:"items"`
	TotalAmount     float64                  `json:"total_amount"`
	Priority        int                      `json:"priority"`
}

// StatusUpdateMessage is the message published by the Kitchen Worker to the notifications_fanout.
type StatusUpdateMessage struct {
	OrderNumber         string    `json:"order_number"`
	OldStatus           string    `json:"old_status"`
	NewStatus           string    `json:"new_status"`
	ChangedBy           string    `json:"changed_by"`
	Timestamp           time.Time `json:"timestamp"`
	EstimatedCompletion time.Time `json:"estimated_completion"`
}
