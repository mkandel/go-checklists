package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkandel/go-checklists/internal/notify"
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
	hub  *notify.Hub
}

// NewStore creates a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, db: pool}
}

// Ping reports whether the underlying connection pool can reach the
// database, for use by a health-check endpoint.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// SetNotifyHub wires h into the store so NotificationRepo.Create publishes a
// wake-up to h after each successful insert. Optional — a Store with no hub
// (the default, used by tests/scripts that don't serve SSE) just skips
// publishing.
func (s *Store) SetNotifyHub(h *notify.Hub) {
	s.hub = h
}

// publish wakes any SSE subscriber for (tenantID, userID), a no-op if no hub
// is wired in.
func (s *Store) publish(tenantID, userID int64) {
	if s.hub != nil {
		s.hub.Publish(tenantID, userID)
	}
}

func (s *Store) Tenants() *TenantRepo     { return &TenantRepo{db: s.db} }
func (s *Store) Users() *UserRepo         { return &UserRepo{db: s.db} }
func (s *Store) Sessions() *SessionRepo   { return &SessionRepo{db: s.db} }
func (s *Store) Groups() *GroupRepo       { return &GroupRepo{db: s.db} }
func (s *Store) Templates() *TemplateRepo { return &TemplateRepo{db: s.db} }
func (s *Store) Events() *EventRepo       { return &EventRepo{db: s.db} }
func (s *Store) Notifications() *NotificationRepo {
	return &NotificationRepo{db: s.db, onCreate: s.publish}
}

func (s *Store) PasswordResetTokens() *PasswordResetTokenRepo {
	return &PasswordResetTokenRepo{db: s.db}
}
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

	txStore := &Store{pool: s.pool, db: tx, hub: s.hub}
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
