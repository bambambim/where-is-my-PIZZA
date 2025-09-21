package trackingservice

import (
	"context"
	"errors"

	"where-is-my-PIZZA/internal/domain"
	"where-is-my-PIZZA/internal/storage/postgres"
)

type Repository interface {
	GetOrderByNumber(ctx context.Context, orderNumber string) (*domain.Order, error)
	GetOrderHistory(ctx context.Context, orderNumber string) ([]domain.OrderStatusLog, error)
	GetAllWorkers(ctx context.Context) ([]domain.Worker, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

var ErrOrderNotFound = errors.New("order not found")

func (s *Service) GetOrderStatus(ctx context.Context, orderNumber string) (*domain.Order, error) {
	order, err := s.repo.GetOrderByNumber(ctx, orderNumber)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return order, nil
}

func (s *Service) GetOrderHistory(ctx context.Context, orderNumber string) ([]domain.OrderStatusLog, error) {
	history, err := s.repo.GetOrderHistory(ctx, orderNumber)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return history, nil
}

func (s *Service) GetWorkersStatus(ctx context.Context) ([]domain.Worker, error) {
	return s.repo.GetAllWorkers(ctx)
}
