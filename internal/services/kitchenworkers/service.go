package kitchenworkers

import (
	"context"
	"github.com/rabbitmq/amqp091-go"
	"sync"
	"time"
	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"
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
	specializedTypes map[string]struct{} // Using a map for efficient lookups
	heartbeat        time.Duration
	prefetchCount    int

	repo   Repository
	broker Broker
	log    *logger.Logger
	wg     sync.WaitGroup
}
type Config struct {
	Name              string
	OrderTypes        string // Comma-separated
	HeartbeatInterval int
	PrefetchCount     int
	Repo              Repository
	Broker            Broker
	Log               *logger.Logger
}
