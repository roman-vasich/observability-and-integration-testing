package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type Order struct {
	ID         string
	CustomerID string
	Status     string
	TotalCents int64
	Note       string
}

type OrderPatch struct {
	Status     *string
	TotalCents *int64
	Note       *string
}

//go:generate go run -mod=mod go.uber.org/mock/mockgen -mock_names OrderRepository=OrderRepositoryMock -destination=./mock/mock_repository.go -package=mock . OrderRepository
type OrderRepository interface {
	CreateMut(ctx context.Context, order Order) error
	UpdateMut(ctx context.Context, id string, patch OrderPatch) error
	GetByID(ctx context.Context, id string) (Order, error)
}

type PostgresOrderRepo struct {
	db *sql.DB
}

func (r PostgresOrderRepo) initSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS orders (
			id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			status TEXT NOT NULL,
			total_cents BIGINT NOT NULL,
			note TEXT NOT NULL DEFAULT ''
		)
	`)
	return err
}

func (r PostgresOrderRepo) CreateMut(ctx context.Context, order Order) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO orders (id, customer_id, status, total_cents, note)
		VALUES ($1, $2, $3, $4, $5)
	`, order.ID, order.CustomerID, order.Status, order.TotalCents, order.Note)
	return err
}

func (r PostgresOrderRepo) UpdateMut(ctx context.Context, id string, patch OrderPatch) error {
	setClauses := make([]string, 0, 3)
	args := make([]any, 0, 4)

	if patch.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, *patch.Status)
	}
	if patch.TotalCents != nil {
		setClauses = append(setClauses, fmt.Sprintf("total_cents = $%d", len(args)+1))
		args = append(args, *patch.TotalCents)
	}
	if patch.Note != nil {
		setClauses = append(setClauses, fmt.Sprintf("note = $%d", len(args)+1))
		args = append(args, *patch.Note)
	}
	if len(setClauses) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf(`UPDATE orders SET %s WHERE id = $%d`, strings.Join(setClauses, ", "), len(args))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r PostgresOrderRepo) GetByID(ctx context.Context, id string) (Order, error) {
	var order Order
	err := r.db.QueryRowContext(ctx, `
		SELECT id, customer_id, status, total_cents, note
		FROM orders
		WHERE id = $1
	`, id).Scan(&order.ID, &order.CustomerID, &order.Status, &order.TotalCents, &order.Note)
	return order, err
}
