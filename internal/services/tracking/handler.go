package trackingservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"
)

type Tracker interface {
	GetOrderStatus(ctx context.Context, orderNumber string) (*domain.Order, error)
	GetOrderHistory(ctx context.Context, orderNumber string) ([]domain.OrderStatusLog, error)
	GetWorkersStatus(ctx context.Context) ([]domain.Worker, error)
}

type Handler struct {
	svc Tracker
	log *logger.Logger
}

func NewHandler(svc Tracker, log *logger.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	orderNumber := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/orders/"), "/status")
	h.log.Debug("request_received", "GetOrderStatus request", requestID)

	order, err := h.svc.GetOrderStatus(r.Context(), orderNumber)
	if err != nil {
		handleError(w, err, "db_query_failed", "Failed to get order status", requestID, h.log)
		return
	}

	resp := map[string]interface{}{
		"order_number":   order.Number,
		"current_status": order.Status,
		"updated_at":     order.UpdatedAt,
		"processed_by":   order.ProcessedBy,
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetOrderHistory(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	orderNumber := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/orders/"), "/history")
	h.log.Debug("request_received", "GetOrderHistory request", requestID)

	history, err := h.svc.GetOrderHistory(r.Context(), orderNumber)
	if err != nil {
		handleError(w, err, "db_query_failed", "Failed to get order history", requestID, h.log)
		return
	}

	type historyItem struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
		ChangedBy string    `json:"changed_by"`
	}
	resp := make([]historyItem, len(history))
	for i, item := range history {
		resp[i] = historyItem{
			Status:    item.Status,
			Timestamp: item.ChangedAt,
			ChangedBy: item.ChangedBy.String,
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetWorkersStatus(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	h.log.Debug("request_received", "GetWorkersStatus request", requestID)

	workers, err := h.svc.GetWorkersStatus(r.Context())
	if err != nil {
		handleError(w, err, "db_query_failed", "Failed to get workers status", requestID, h.log)
		return
	}

	type workerStatus struct {
		WorkerName      string    `json:"worker_name"`
		Status          string    `json:"status"`
		OrdersProcessed int       `json:"orders_processed"`
		LastSeen        time.Time `json:"last_seen"`
	}
	resp := make([]workerStatus, len(workers))
	for i, w := range workers {
		resp[i] = workerStatus{
			WorkerName:      w.Name,
			Status:          w.Status,
			OrdersProcessed: w.OrdersProcessed,
			LastSeen:        w.LastSeen,
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

func handleError(w http.ResponseWriter, err error, action, msg, reqID string, log *logger.Logger) {
	if errors.Is(err, ErrOrderNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
	} else {
		log.Error(err, action, msg, reqID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
