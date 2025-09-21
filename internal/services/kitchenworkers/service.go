package kitchenworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"
	"where-is-my-PIZZA/internal/storage/postgres"

	"github.com/rabbitmq/amqp091-go"
)

type Repository interface {
	GetWorkerByName(ctx context.Context, name string) (*domain.Worker, error)
	RegisterWorker(ctx context.Context, worker *domain.Worker) error
	UpdateWorkerStatus(ctx context.Context, name, status string) error
	UpdateWorkerHeartbeat(ctx context.Context, name string) error
	GetOrderByNumber(ctx context.Context, orderNumber string) (*domain.Order, error)
	ProcessOrderCooking(ctx context.Context, orderNumber, workerName string) error
	ProcessOrderReady(ctx context.Context, orderNumber string) error
}

type Broker interface {
	ConsumeOrders(prefetchCount int, routingKeys []string) (<-chan amqp091.Delivery, error)
	PublishStatusUpdate(ctx context.Context, msg domain.StatusUpdateMessage) error
}

type Worker struct {
	name             string
	specializedTypes map[string]struct{}
	heartbeat        time.Duration
	prefetchCount    int
	repo             Repository
	broker           Broker
	log              *logger.Logger
	wg               sync.WaitGroup
}

type Config struct {
	Name              string
	OrderTypes        string
	HeartbeatInterval int
	PrefetchCount     int
	Repo              Repository
	Broker            Broker
	Log               *logger.Logger
}

func NewWorker(cfg Config) (*Worker, error) {
	specializedTypes := make(map[string]struct{})
	if cfg.OrderTypes != "" {
		types := strings.Split(cfg.OrderTypes, ",")
		for _, t := range types {
			specializedTypes[strings.TrimSpace(t)] = struct{}{}
		}
	}

	return &Worker{
		name:             cfg.Name,
		specializedTypes: specializedTypes,
		heartbeat:        time.Duration(cfg.HeartbeatInterval) * time.Second,
		prefetchCount:    cfg.PrefetchCount,
		repo:             cfg.Repo,
		broker:           cfg.Broker,
		log:              cfg.Log,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker_startup", fmt.Sprintf("Starting kitchen worker: %s", w.name))

	if err := w.register(ctx); err != nil {
		return err
	}

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	w.wg.Add(1)
	go w.startHeartbeat(heartbeatCtx)

	routingKeys := w.getRoutingKeys()
	msgs, err := w.broker.ConsumeOrders(w.prefetchCount, routingKeys)
	if err != nil {
		w.log.Error(err, "rabbitmq_consume_failed", "Failed to start consuming order messages")
		return err
	}
	w.log.Info("consumer_started", fmt.Sprintf("Worker waiting for orders. Routing keys: %v", routingKeys))

	w.processMessages(ctx, msgs)

	w.shutdown()
	return nil
}

func (w *Worker) register(ctx context.Context) error {
	existingWorker, err := w.repo.GetWorkerByName(ctx, w.name)
	if err != nil {
		if !errors.Is(err, postgres.ErrNotFound) {
			w.log.Error(err, "worker_registration_failed", "Failed to check for existing worker")
			return err
		}

		// Determine worker type based on specialization
		workerType := "kitchen" // default for general workers
		if len(w.specializedTypes) > 0 {
			types := make([]string, 0, len(w.specializedTypes))
			for t := range w.specializedTypes {
				types = append(types, t)
			}
			workerType = strings.Join(types, ",") // e.g., "dine_in" or "dine_in,delivery"
		}

		newWorker := &domain.Worker{Name: w.name, Type: workerType, Status: "online"}
		if err := w.repo.RegisterWorker(ctx, newWorker); err != nil {
			w.log.Error(err, "worker_registration_failed", "Failed to insert new worker record")
			return err
		}
		w.log.Info("worker_registered", "New worker successfully registered")
		return nil
	}

	if existingWorker.Status == "online" {
		err := fmt.Errorf("a worker with name '%s' is already online", w.name)
		w.log.Error(err, "worker_registration_failed", "Duplicate worker detected")
		return err
	}

	if err := w.repo.UpdateWorkerStatus(ctx, w.name, "online"); err != nil {
		w.log.Error(err, "worker_registration_failed", "Failed to update status for existing offline worker")
		return err
	}
	w.log.Info("worker_registered", "Re-registered existing offline worker as online")
	return nil
}

func (w *Worker) getRoutingKeys() []string {
	if len(w.specializedTypes) == 0 {
		return []string{"kitchen.#"}
	}
	keys := make([]string, 0, len(w.specializedTypes))
	for t := range w.specializedTypes {
		keys = append(keys, fmt.Sprintf("kitchen.%s.#", t))
	}
	return keys
}

func (w *Worker) processMessages(ctx context.Context, msgs <-chan amqp091.Delivery) {
	for {
		select {
		case <-ctx.Done():
			w.log.Info("shutdown_signal", "Context canceled, stopping message processing")
			return
		case msg, ok := <-msgs:
			if !ok {
				w.log.Info("channel_closed", "Message channel closed by broker")
				return
			}
			w.wg.Add(1)
			go w.handleMessage(ctx, msg)
		}
	}
}

func (w *Worker) handleMessage(ctx context.Context, msg amqp091.Delivery) {
	defer w.wg.Done()

	var orderMsg domain.OrderMessage
	if err := json.Unmarshal(msg.Body, &orderMsg); err != nil {
		w.log.Error(err, "message_unmarshal_failed", "Cannot parse order message, sending to DLQ")
		msg.Nack(false, false)
		return
	}

	orderNum := orderMsg.OrderNumber
	w.log.Debug("order_received", fmt.Sprintf("Picked up order %s", orderNum), orderNum)

	if len(w.specializedTypes) > 0 {
		if _, ok := w.specializedTypes[orderMsg.OrderType]; !ok {
			w.log.Debug("order_requeued", fmt.Sprintf("Worker not specialized for '%s', requeueing", orderMsg.OrderType), orderNum)
			msg.Nack(false, true)
			return
		}
	}

	currentOrder, err := w.repo.GetOrderByNumber(ctx, orderNum)
	if err != nil {
		w.log.Error(err, "db_query_failed", fmt.Sprintf("Failed to get status for order %s", orderNum), orderNum)
		msg.Nack(false, true)
		return
	}
	if currentOrder.Status != "received" {
		w.log.Debug("order_already_processed", fmt.Sprintf("Order %s already in '%s' status, skipping", orderNum, currentOrder.Status), orderNum)
		msg.Ack(false)
		return
	}

	if err := w.repo.ProcessOrderCooking(ctx, orderNum, w.name); err != nil {
		w.log.Error(err, "db_transaction_failed", fmt.Sprintf("Failed to set order %s to 'cooking'", orderNum), orderNum)
		msg.Nack(false, true)
		return
	}
	w.broker.PublishStatusUpdate(ctx, w.newStatusUpdate(orderMsg, "received", "cooking"))
	w.log.Debug("order_processing_started", fmt.Sprintf("Order %s status set to 'cooking'", orderNum), orderNum)

	cookingTime := w.getCookingTime(orderMsg.OrderType)
	select {
	case <-time.After(cookingTime):
	case <-ctx.Done():
		w.log.Info("shutdown_during_cooking", fmt.Sprintf("Shutdown signaled while cooking order %s, requeueing", orderNum), orderNum)
		msg.Nack(false, true)
		return
	}

	if err := w.repo.ProcessOrderReady(ctx, orderNum); err != nil {
		w.log.Error(err, "db_transaction_failed", fmt.Sprintf("Failed to set order %s to 'ready'", orderNum), orderNum)
		msg.Nack(false, true)
		return
	}
	w.broker.PublishStatusUpdate(ctx, w.newStatusUpdate(orderMsg, "cooking", "ready"))
	w.log.Info("order_completed", fmt.Sprintf("Order %s is now 'ready'", orderNum), orderNum)

	msg.Ack(false)
}

func (w *Worker) startHeartbeat(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.repo.UpdateWorkerHeartbeat(ctx, w.name); err != nil {
				w.log.Error(err, "heartbeat_failed", "Failed to send heartbeat")
			} else {
				w.log.Debug("heartbeat_sent", "Heartbeat sent")
			}
		case <-ctx.Done():
			w.log.Info("heartbeat_stopped", "Stopping heartbeat")
			return
		}
	}
}

func (w *Worker) shutdown() {
	w.log.Info("graceful_shutdown", "Waiting for in-flight tasks...")
	w.wg.Wait()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.repo.UpdateWorkerStatus(shutdownCtx, w.name, "offline"); err != nil {
		w.log.Error(err, "shutdown_failed", "Failed to update worker status to offline")
	}

	w.log.Info("graceful_shutdown", "Worker has shut down gracefully")
}

func (w *Worker) getCookingTime(orderType string) time.Duration {
	switch orderType {
	case "dine_in":
		return 8 * time.Second
	case "takeout":
		return 10 * time.Second
	case "delivery":
		return 12 * time.Second
	default:
		return 10 * time.Second
	}
}

func (w *Worker) newStatusUpdate(order domain.OrderMessage, old, new string) domain.StatusUpdateMessage {
	return domain.StatusUpdateMessage{
		OrderNumber:         order.OrderNumber,
		OldStatus:           old,
		NewStatus:           new,
		ChangedBy:           w.name,
		Timestamp:           time.Now().UTC(),
		EstimatedCompletion: time.Now().UTC().Add(w.getCookingTime(order.OrderType)),
	}
}
