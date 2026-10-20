package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type txManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) TxManager {
	return &txManager{
		pool: pool,
	}
}

func (m *txManager) Do(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	if fn == nil {
		return errors.New("transaction callback must not be nil")
	}

	if _, ok := executorFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := ContextWithExecutor(ctx, tx)

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = rollback(tx)
			panic(recovered)
		}
	}()

	if err := fn(txCtx); err != nil {
		rollbackErr := rollback(tx)
		if rollbackErr != nil {
			return errors.Join(
				err,
				fmt.Errorf("rollback transaction: %w", rollbackErr),
			)
		}

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

const rollbackTimeout = 5 * time.Second

func rollback(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		rollbackTimeout,
	)
	defer cancel()

	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}

	return err
}
