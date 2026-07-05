package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
	"github.com/mkandel/go-checklists/internal/mail"
	"github.com/mkandel/go-checklists/internal/store/postgres"
)

// emailWorkerInterval is how often the delivery worker sweeps for pending
// notification emails. emailBatchSize bounds how many it attempts per tick.
// maxEmailAttempts is how many failed attempts a notification gets before
// it's given up on (roughly emailWorkerInterval * maxEmailAttempts of
// retrying — enough for a transient SMTP hiccup at this volume, not worth
// backoff logic).
const (
	emailWorkerInterval = 60 * time.Second
	emailBatchSize      = 50
	maxEmailAttempts    = 10
)

// runEmailDelivery periodically sends pending notification emails until ctx
// is canceled, then signals wg it's done so main can exit cleanly. Email
// I/O happens entirely outside any DB transaction: the notification row is
// already committed by the time this worker sees it, so a slow or failing
// SMTP call never holds a DB transaction open.
func runEmailDelivery(ctx context.Context, wg *sync.WaitGroup, store *postgres.Store) {
	defer wg.Done()

	ticker := time.NewTicker(emailWorkerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer recoverTick("email delivery")
				deliverPendingEmails(ctx, store)
			}()
		}
	}
}

func deliverPendingEmails(ctx context.Context, store *postgres.Store) {
	pending, err := store.Notifications().ListPendingEmail(ctx, emailBatchSize)
	if err != nil {
		log.Printf("email delivery: list pending: %v", err)
		return
	}

	for _, n := range pending {
		if err := deliverOne(ctx, store, n); err != nil {
			log.Printf("email delivery: notification %d: %v", n.ID, err)
		}
	}
}

func deliverOne(ctx context.Context, store *postgres.Store, n domain.Notification) error {
	recipient, err := store.Users().GetByID(ctx, n.RecipientUserID)
	if err != nil {
		return fmt.Errorf("get recipient: %w", err)
	}
	tenant, err := store.Tenants().GetByID(ctx, n.TenantID)
	if err != nil {
		return fmt.Errorf("get tenant: %w", err)
	}

	switch {
	case tenant.SMTPHost == nil:
		log.Printf("email delivery: skipping notification %d: tenant %d has no SMTP config", n.ID, tenant.ID)
		return store.Notifications().MarkEmailSkipped(ctx, n.ID)
	case recipient.Email == nil:
		log.Printf("email delivery: skipping notification %d: recipient %d has no email", n.ID, recipient.ID)
		return store.Notifications().MarkEmailSkipped(ctx, n.ID)
	case !recipient.IsActive:
		log.Printf("email delivery: skipping notification %d: recipient %d is deactivated", n.ID, recipient.ID)
		return store.Notifications().MarkEmailSkipped(ctx, n.ID)
	}

	cfg := mail.SMTPConfig{
		Host:        *tenant.SMTPHost,
		Username:    strPtrValue(tenant.SMTPUsername),
		Password:    tenant.SMTPPassword,
		FromAddress: strPtrValue(tenant.SMTPFromAddress),
	}
	if tenant.SMTPPort != nil {
		cfg.Port = *tenant.SMTPPort
	}
	msg := mail.Message{
		To:      *recipient.Email,
		Subject: "Checklists notification",
		Body:    n.Message,
	}

	if err := mail.Send(cfg, msg); err != nil {
		log.Printf("email delivery: notification %d failed: %v", n.ID, err)
		return store.Notifications().MarkEmailFailed(ctx, n.ID, err.Error(), maxEmailAttempts)
	}
	return store.Notifications().MarkEmailSent(ctx, n.ID, time.Now())
}

func strPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
