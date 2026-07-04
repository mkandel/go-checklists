package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// EventRepo is the Postgres-backed implementation of domain.EventRepo,
// appending to the append-only checklist_events audit log.
type EventRepo struct {
	db dbtx
}

var _ domain.EventRepo = (*EventRepo)(nil)

func (r *EventRepo) Append(ctx context.Context, events []domain.Event) error {
	for _, e := range events {
		detailJSON := "{}"
		if len(e.Detail) > 0 {
			b, err := json.Marshal(e.Detail)
			if err != nil {
				return fmt.Errorf("postgres: marshal event detail: %w", err)
			}
			detailJSON = string(b)
		}
		_, err := r.db.Exec(ctx,
			`INSERT INTO checklist_events (checklist_id, item_id, actor_user_id, action, detail)
			 VALUES ($1, $2, $3, $4, $5::jsonb)`,
			e.ChecklistID, e.ItemID, e.ActorUserID, e.Action, detailJSON,
		)
		if err != nil {
			return fmt.Errorf("postgres: append event: %w", err)
		}
	}
	return nil
}
