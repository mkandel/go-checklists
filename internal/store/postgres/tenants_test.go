//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/mkandel/go-checklists/internal/domain"
)

func mustCreateTenant(t *testing.T, name, slug string) *domain.Tenant {
	t.Helper()
	tenant := &domain.Tenant{Name: name, Slug: slug}
	if err := testStore.Tenants().Create(context.Background(), tenant); err != nil {
		t.Fatalf("create tenant %s: %v", slug, err)
	}
	return tenant
}

func TestTenantRepo_UpdateMailConfig(t *testing.T) {
	ctx := context.Background()
	tenant := mustCreateTenant(t, "Mail Test Tenant", uniqueName(t, "mail-tenant"))

	cfg := domain.TenantMailConfig{
		Host:        "smtp-relay.brevo.com",
		Port:        587,
		Username:    "smtp-user",
		Password:    "secret",
		FromAddress: "notifications@example.com",
	}
	if err := testStore.Tenants().UpdateMailConfig(ctx, tenant.ID, cfg); err != nil {
		t.Fatalf("update mail config: %v", err)
	}

	got, err := testStore.Tenants().GetByID(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.SMTPHost == nil || *got.SMTPHost != cfg.Host {
		t.Fatalf("expected SMTPHost %q, got %v", cfg.Host, got.SMTPHost)
	}
	if got.SMTPPort == nil || *got.SMTPPort != cfg.Port {
		t.Fatalf("expected SMTPPort %d, got %v", cfg.Port, got.SMTPPort)
	}
	if got.SMTPUsername == nil || *got.SMTPUsername != cfg.Username {
		t.Fatalf("expected SMTPUsername %q, got %v", cfg.Username, got.SMTPUsername)
	}
	if got.SMTPPassword != cfg.Password {
		t.Fatalf("expected SMTPPassword %q, got %q", cfg.Password, got.SMTPPassword)
	}
	if got.SMTPFromAddress == nil || *got.SMTPFromAddress != cfg.FromAddress {
		t.Fatalf("expected SMTPFromAddress %q, got %v", cfg.FromAddress, got.SMTPFromAddress)
	}
}

func TestTenantRepo_UpdateMailConfigEmptyPasswordKeepsExisting(t *testing.T) {
	ctx := context.Background()
	tenant := mustCreateTenant(t, "Mail Test Tenant", uniqueName(t, "mail-tenant"))

	initial := domain.TenantMailConfig{
		Host: "smtp-relay.brevo.com", Port: 587, Username: "smtp-user",
		Password: "original-secret", FromAddress: "notifications@example.com",
	}
	if err := testStore.Tenants().UpdateMailConfig(ctx, tenant.ID, initial); err != nil {
		t.Fatalf("initial update mail config: %v", err)
	}

	updated := domain.TenantMailConfig{
		Host: "smtp-relay.brevo.com", Port: 465, Username: "smtp-user-2",
		Password: "", FromAddress: "notifications2@example.com",
	}
	if err := testStore.Tenants().UpdateMailConfig(ctx, tenant.ID, updated); err != nil {
		t.Fatalf("second update mail config: %v", err)
	}

	got, err := testStore.Tenants().GetByID(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	if got.SMTPPassword != initial.Password {
		t.Fatalf("expected password to remain %q after empty-password update, got %q", initial.Password, got.SMTPPassword)
	}
	if got.SMTPPort == nil || *got.SMTPPort != updated.Port {
		t.Fatalf("expected SMTPPort to update to %d, got %v", updated.Port, got.SMTPPort)
	}
	if got.SMTPUsername == nil || *got.SMTPUsername != updated.Username {
		t.Fatalf("expected SMTPUsername to update to %q, got %v", updated.Username, got.SMTPUsername)
	}
}
