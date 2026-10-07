package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
)

// EmailSender is the seam between OPS-068b's OTP flows and whatever
// actually delivers the email — an interface, not a concrete type, so
// tests can swap in a fake instead of making real calls to Resend or an
// SMTP server, the same reason GoogleOAuthClient (OPS-068a) and every
// domain store in this codebase are built behind an interface.
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// resendAPIURL is Resend's REST endpoint. Called directly over HTTPS
// rather than via their official SDK — one more POST + JSON encode/decode
// isn't worth a dependency, the same "minimal dependency surface"
// reasoning behind calling Google's userinfo endpoint directly in
// OPS-068a instead of a full OIDC library.
const resendAPIURL = "https://api.resend.com/emails"

// ResendEmailSender sends via Resend's API. Selected at startup
// (router.New) whenever RESEND_API_KEY is set — takes priority over SMTP
// if both happen to be configured.
type ResendEmailSender struct {
	apiKey     string
	from       string
	httpClient *http.Client
}

func NewResendEmailSender(apiKey, from string) ResendEmailSender {
	return ResendEmailSender{apiKey: apiKey, from: from, httpClient: http.DefaultClient}
}

func (s ResendEmailSender) Send(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(map[string]any{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"text":    body,
	})
	if err != nil {
		return fmt.Errorf("auth: encode resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("auth: build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth: call resend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("auth: resend returned %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

// SMTPEmailSender sends via a plain SMTP server using only the standard
// library (net/smtp) — zero new dependency. Selected at startup only when
// RESEND_API_KEY is empty and SMTP_HOST is set — this is a startup-time
// choice, not a runtime failover: whichever is configured when the
// process starts is what's used for the life of that process, the same
// "absent config picks the path" pattern as DATABASE_URL and
// GOOGLE_CLIENT_ID. A true try-Resend-then-fall-back-to-SMTP-on-failure
// policy would need retry/circuit-breaker logic this starter doesn't
// otherwise have anywhere, for a problem (Resend being down) this
// simpler, consistent pattern doesn't need to solve.
type SMTPEmailSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPEmailSender(host, port, username, password, from string) SMTPEmailSender {
	return SMTPEmailSender{host: host, port: port, username: username, password: password, from: from}
}

func (s SMTPEmailSender) Send(_ context.Context, to, subject, body string) error {
	addr := s.host + ":" + s.port

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", s.from, to, subject, body)

	if err := smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("auth: send smtp mail: %w", err)
	}

	return nil
}
