package orderservice

import (
	"context"
	"database/sql"
	"errors"

	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/logger"
)

var (
	ErrInvalidOrderType       = errors.New("order type must be 'dine_in', 'takeout', or 'delivery'")
	ErrMissingItems           = errors.New("order must contain between 1 and 20 items")
	ErrInvalidItems           = errors.New("order items contain invalid data")
	ErrMissingTableNumber     = errors.New("table_number is required for dine_in orders")
	ErrMissingDeliveryAddress = errors.New("delivery_address is required for delivery orders")
	ErrInvalidTableNumber     = errors.New("table_number must not be provided for non-dine_in orders")
	ErrInvalidDeliveryAddress = errors.New("delivery_address must not be provided for non-delivery orders")
	ErrTableNumberRange       = errors.New("table_number must be between 1 and 100")
	ErrDeliveryAddressLength  = errors.New("delivery_address must be at least 10 characters long")
)

type Repository interface {
	CreateOrderInTransaction(ctx context.Context, order *domain.Order, items []domain.OrderItem) (*domain.Order, error)
}

type Broker interface {
	PublishOrder(ctx context.Context, msg domain.OrderMessage) error
}

type Service struct {
	repo   Repository
	broker Broker
	log    *logger.Logger
}

func NewService(repo Repository, broker Broker, log *logger.Logger) *Service {
	return &Service{repo: repo, broker: broker, log: log}
}

func (s *Service) CreateOrder(ctx context.Context, req domain.CreateOrderRequest) (*domain.Order, error) {
	// 1. Validate input
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	// 2. Process data
	totalAmount := calculateTotalAmount(req.Items)
	priority := assignPriority(totalAmount)

	order := &domain.Order{
		CustomerName: req.CustomerName,
		Type:         req.OrderType,
		TotalAmount:  totalAmount,
		Priority:     priority,
	}
	if req.TableNumber != nil {
		order.TableNumber = sql.NullInt32{Int32: int32(*req.TableNumber), Valid: true}
	}
	if req.DeliveryAddress != nil {
		order.DeliveryAddress = sql.NullString{String: *req.DeliveryAddress, Valid: true}
	}

	orderItems := make([]domain.OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = domain.OrderItem{
			Name:     item.Name,
			Quantity: item.Quantity,
			Price:    item.Price,
		}
	}

	// 3. Database Transaction
	// 3. Database Transaction
	createdOrder, err := s.repo.CreateOrderInTransaction(ctx, order, orderItems)
	if err != nil {
		// If this fails, the function should stop and NOT publish.
		s.log.Error(err, "db_transaction_failed", "failed to create order")
		return nil, err
	}

	// 4. Publish Message (ONLY happens if the above err is nil)

	msg := domain.OrderMessage{
		OrderNumber:     createdOrder.Number,
		CustomerName:    createdOrder.CustomerName,
		OrderType:       createdOrder.Type,
		TableNumber:     req.TableNumber,
		DeliveryAddress: req.DeliveryAddress,
		Items:           req.Items,
		TotalAmount:     createdOrder.TotalAmount,
		Priority:        createdOrder.Priority,
	}
	if err := s.broker.PublishOrder(ctx, msg); err != nil {
		s.log.Error(err, "rabbitmq_publish_failed", "Failed to publish order message", createdOrder.Number)
		return nil, err
	}
	s.log.Info("order_published", "Order successfully published to message broker", createdOrder.Number)

	return createdOrder, nil
}

func (s *Service) validateRequest(req domain.CreateOrderRequest) error {
	// Validate order type
	if req.OrderType != "dine_in" && req.OrderType != "takeout" && req.OrderType != "delivery" {
		return ErrInvalidOrderType
	}

	// Validate items
	if len(req.Items) < 1 || len(req.Items) > 20 {
		return ErrMissingItems
	}

	// Conditional validation based on order type
	switch req.OrderType {
	case "dine_in":
		// dine_in requires table_number and must NOT have delivery_address
		if req.TableNumber == nil {
			return ErrMissingTableNumber
		}
		// Validate table_number range (1-100)
		if *req.TableNumber < 1 || *req.TableNumber > 100 {
			return ErrTableNumberRange
		}
		if req.DeliveryAddress != nil {
			return ErrInvalidDeliveryAddress
		}
	case "delivery":
		// delivery requires delivery_address and must NOT have table_number
		if req.DeliveryAddress == nil || *req.DeliveryAddress == "" {
			return ErrMissingDeliveryAddress
		}
		// Validate delivery_address length (minimum 10 characters)
		if len(*req.DeliveryAddress) < 10 {
			return ErrDeliveryAddressLength
		}
		if req.TableNumber != nil {
			return ErrInvalidTableNumber
		}
	case "takeout":
		// takeout must NOT have table_number or delivery_address
		if req.TableNumber != nil {
			return ErrInvalidTableNumber
		}
		if req.DeliveryAddress != nil {
			return ErrInvalidDeliveryAddress
		}
	}

	return nil
}

func calculateTotalAmount(items []domain.CreateOrderItemRequest) float64 {
	var total float64
	for _, item := range items {
		total += float64(item.Quantity) * item.Price
	}
	// A simple way to round to 2 decimal places
	return float64(int(total*100)) / 100
}

func assignPriority(total float64) int {
	if total > 100 {
		return 10
	}
	if total >= 50 {
		return 5
	}
	return 1
}
