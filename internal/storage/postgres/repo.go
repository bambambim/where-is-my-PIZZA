package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"where-is-my-PIZZA/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CreateOrderInTransaction creates a new order and its associated items within a single DB transaction.
func (r *Repository) CreateOrderInTransaction(ctx context.Context, order *domain.Order, items []domain.OrderItem) (*domain.Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var sequence int
	if err := tx.QueryRow(ctx, "SELECT get_next_daily_order_sequence()").Scan(&sequence); err != nil {
		return nil, fmt.Errorf("failed to get next order sequence: %w", err)
	}
	order.Number = fmt.Sprintf("ORD_%s_%03d", time.Now().UTC().Format("20060102"), sequence)

	orderSQL := `
        INSERT INTO orders (number, customer_name, type, table_number, delivery_address, total_amount, priority, status)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING id, created_at, updated_at, status
    `
	err = tx.QueryRow(ctx, orderSQL,
		order.Number, order.CustomerName, order.Type, order.TableNumber, order.DeliveryAddress, order.TotalAmount, order.Priority, "received",
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt, &order.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to insert into orders table: %w", err)
	}

	batch := &pgx.Batch{}
	itemSQL := "INSERT INTO order_items (order_id, name, quantity, price) VALUES ($1, $2, $3, $4)"
	for _, item := range items {
		batch.Queue(itemSQL, order.ID, item.Name, item.Quantity, item.Price)
	}
	br := tx.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("failed on batch insert for order_items: %w", err)
	}

	logSQL := "INSERT INTO order_status_log (order_id, status, changed_by) VALUES ($1, $2, $3)"
	if _, err := tx.Exec(ctx, logSQL, order.ID, "received", "order-service"); err != nil {
		return nil, fmt.Errorf("failed to insert into order_status_log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return order, nil
}

// (The rest of your repository functions: GetOrderByNumber, GetWorkerByName, etc.)
// ... (keep the rest of the file as it was)
func (r *Repository) GetOrderByNumber(ctx context.Context, orderNumber string) (*domain.Order, error) {
	var order domain.Order
	query := "SELECT id, created_at, updated_at, number, customer_name, type, table_number, delivery_address, total_amount, priority, status, processed_by, completed_at FROM orders WHERE number = $1"
	err := r.db.QueryRow(ctx, query, orderNumber).Scan(
		&order.ID, &order.CreatedAt, &order.UpdatedAt, &order.Number, &order.CustomerName, &order.Type, &order.TableNumber, &order.DeliveryAddress, &order.TotalAmount, &order.Priority, &order.Status, &order.ProcessedBy, &order.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (r *Repository) GetOrderHistory(ctx context.Context, orderNumber string) ([]domain.OrderStatusLog, error) {
	query := `
        SELECT l.status, l.changed_at, l.changed_by
        FROM order_status_log l
        JOIN orders o ON l.order_id = o.id
        WHERE o.number = $1
        ORDER BY l.changed_at ASC
    `
	rows, err := r.db.Query(ctx, query, orderNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []domain.OrderStatusLog
	for rows.Next() {
		var log domain.OrderStatusLog
		if err := rows.Scan(&log.Status, &log.ChangedAt, &log.ChangedBy); err != nil {
			return nil, err
		}
		history = append(history, log)
	}
	if len(history) == 0 {
		var exists int
		err := r.db.QueryRow(ctx, "SELECT 1 FROM orders WHERE number = $1", orderNumber).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
	}
	return history, nil
}

func (r *Repository) GetWorkerByName(ctx context.Context, name string) (*domain.Worker, error) {
	var worker domain.Worker
	query := "SELECT id, created_at, name, type, status, last_seen, orders_processed FROM workers WHERE name = $1"
	err := r.db.QueryRow(ctx, query, name).Scan(&worker.ID, &worker.CreatedAt, &worker.Name, &worker.Type, &worker.Status, &worker.LastSeen, &worker.OrdersProcessed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &worker, nil
}

func (r *Repository) RegisterWorker(ctx context.Context, worker *domain.Worker) error {
	query := "INSERT INTO workers (name, type, status, last_seen) VALUES ($1, $2, $3, NOW())"
	_, err := r.db.Exec(ctx, query, worker.Name, worker.Type, worker.Status)
	return err
}

func (r *Repository) UpdateWorkerStatus(ctx context.Context, name, status string) error {
	query := "UPDATE workers SET status = $1, last_seen = NOW() WHERE name = $2"
	_, err := r.db.Exec(ctx, query, status, name)
	return err
}

func (r *Repository) UpdateWorkerHeartbeat(ctx context.Context, name string) error {
	query := "UPDATE workers SET last_seen = NOW() WHERE name = $1 AND status = 'online'"
	_, err := r.db.Exec(ctx, query, name)
	return err
}

func (r *Repository) GetAllWorkers(ctx context.Context) ([]domain.Worker, error) {
	rows, err := r.db.Query(ctx, "SELECT name, status, orders_processed, last_seen FROM workers ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []domain.Worker
	for rows.Next() {
		var w domain.Worker
		if err := rows.Scan(&w.Name, &w.Status, &w.OrdersProcessed, &w.LastSeen); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (r *Repository) ProcessOrderCooking(ctx context.Context, orderNumber, workerName string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orderID int64
	updateSQL := "UPDATE orders SET status = 'cooking', processed_by = $1, updated_at = NOW() WHERE number = $2 RETURNING id"
	if err := tx.QueryRow(ctx, updateSQL, workerName, orderNumber).Scan(&orderID); err != nil {
		return err
	}

	logSQL := "INSERT INTO order_status_log (order_id, status, changed_by) VALUES ($1, 'cooking', $2)"
	if _, err := tx.Exec(ctx, logSQL, orderID, workerName); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) ProcessOrderReady(ctx context.Context, orderNumber string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orderID int64
	var workerName sql.NullString
	updateSQL := "UPDATE orders SET status = 'ready', completed_at = NOW(), updated_at = NOW() WHERE number = $1 RETURNING id, processed_by"
	if err := tx.QueryRow(ctx, updateSQL, orderNumber).Scan(&orderID, &workerName); err != nil {
		return err
	}

	logSQL := "INSERT INTO order_status_log (order_id, status, changed_by) VALUES ($1, 'ready', $2)"
	if _, err := tx.Exec(ctx, logSQL, orderID, workerName.String); err != nil {
		return err
	}

	if workerName.Valid {
		workerSQL := "UPDATE workers SET orders_processed = orders_processed + 1 WHERE name = $1"
		if _, err := tx.Exec(ctx, workerSQL, workerName.String); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
