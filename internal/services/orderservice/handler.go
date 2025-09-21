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

type OrderCreator interface {
	CreateOrder(ctx context.Context, req domain.CreateOrderRequest) (*domain.Order, error)
}

type Handler struct {
	svc OrderCreator
	log *logger.Logger
}

func NewHand(svc OrderCreator, log *logger.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	h.log.Debug("request_received", "Received create order request", requestID)

	var req domain.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error(err, "validation_failed", "Failed to decode request body", requestID)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	order, err := h.svc.CreateOrder(ctx, req)
	if err != nil {
		if errors.Is(err, ErrInvalidOrderType) ||
			errors.Is(err, ErrMissingItems) ||
			errors.Is(err, ErrMissingTableNumber) ||
			errors.Is(err, ErrMissingDeliveryAddress) ||
			errors.Is(err, ErrInvalidTableNumber) ||
			errors.Is(err, ErrInvalidDeliveryAddress) ||
			errors.Is(err, ErrTableNumberRange) ||
			errors.Is(err, ErrDeliveryAddressLength) {
			h.log.Error(err, "validation_failed", "Order validation failed", requestID)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.log.Error(err, "processing_failed", "Failed to create order", requestID)
		http.Error(w, "Failed to process order", http.StatusInternalServerError)
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
		h.log.Error(err, "response_failed", "Failed to encode response", requestID)
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
