package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbtx is the subset of *pgxpool.Pool and pgx.Tx that repo implementations
// need. Repos are constructed against a dbtx so the same code runs whether
// it's bound to the pool (no transaction) or a transaction started by
// Store.WithTx.
type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store is the entry point to all repositories, backed by a pgx connection
// pool (or, inside WithTx, a single transaction).
type Store struct {
	pool *pgxpool.Pool
	db   dbtx
}

// NewStore creates a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, db: pool}
}

func (s *Store) Tenants() *TenantRepo             { return &TenantRepo{db: s.db} }
func (s *Store) Users() *UserRepo                 { return &UserRepo{db: s.db} }
func (s *Store) Sessions() *SessionRepo           { return &SessionRepo{db: s.db} }
func (s *Store) Groups() *GroupRepo               { return &GroupRepo{db: s.db} }
func (s *Store) Templates() *TemplateRepo         { return &TemplateRepo{db: s.db} }
func (s *Store) Events() *EventRepo               { return &EventRepo{db: s.db} }
func (s *Store) Notifications() *NotificationRepo { return &NotificationRepo{db: s.db} }
func (s *Store) Checklists() *ChecklistRepo {
	return &ChecklistRepo{
		db:            s.db,
		templates:     s.Templates(),
		events:        s.Events(),
		notifications: s.Notifications(),
	}
}

// WithTx runs fn against a Store bound to a single transaction, committing
// on success and rolling back on error (including a panic, which is
// re-raised after rollback).
func (s *Store) WithTx(ctx context.Context, fn func(*Store) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txStore := &Store{pool: s.pool, db: tx}
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
