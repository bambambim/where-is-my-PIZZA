package orderservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"
)

var (
	ErrInvalidOrderType = errors.New("order type must be 'dine_in', 'takeout', or 'delivery'")
	ErrMissingItems     = errors.New("order must contain between 1 and 20 items")
	ErrInvalidItems     = errors.New("order items contain invalid data")
)

type OrderService interface {
	CreateOrder(ctx context.Context, order domain.CreateOrderRequest) (domain.Order, error)
}

type Handler struct {
	service OrderService
	log     *logger.Logger
}

func NewHandler(service OrderService, logger *logger.Logger) *Handler {
	return &Handler{service: service, log: logger}
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	reqID := generateRequestID()
	h.log.Debug("request_received", "Received create order request", reqID)
	var req domain.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error(err, "validation_failed", "Failed to decode request body", reqID)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*10)
	defer cancel()

	order, err := h.service.CreateOrder(ctx, req)
	if err != nil {
		if errors.Is(err, ErrInvalidOrderType) || errors.Is(err, ErrMissingItems) {
			h.log.Error(err, "validation_failed", "Order validation failed", reqID)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Order creation failed", http.StatusInternalServerError)
		return
	}

	resp := domain.CreateOrderResponse{
		OrderNumber: order.Number,
		Status:      order.Status,
		TotalAmount: order.TotalAmount,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error(err, "response_failed", "Failed to encode response")
	}

}

// generateRequestID creates a simple, unique ID for tracing without external dependencies.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
