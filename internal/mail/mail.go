// Package mail sends outbound email over SMTP. It is framework/DB-agnostic:
// no domain or store types here, just wire-level SMTP delivery. Callers
// (the email delivery worker in cmd/checklists-server) translate
// domain.Tenant SMTP config and domain.Notification content into the types
// below.
package mail

import (
	"fmt"
	"net/smtp"
)

// SMTPConfig is a tenant's outbound SMTP server settings.
type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
}

// Message is a single outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Send delivers msg via cfg's SMTP server. It uses net/smtp.SendMail, which
// opportunistically upgrades to TLS (STARTTLS) when the server advertises
// it and authenticates with PlainAuth — sufficient for providers like
// Brevo's smtp-relay.brevo.com:587 without a third-party dependency.
func Send(cfg SMTPConfig, msg Message) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n",
		cfg.FromAddress, msg.To, msg.Subject, msg.Body)
	return smtp.SendMail(addr, auth, cfg.FromAddress, []string{msg.To}, []byte(body))
}
