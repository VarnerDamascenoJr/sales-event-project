package notification

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type IssuedTicket struct {
	ID         string
	TicketName string
	QRCodePNG  []byte
	QRPayload  string
}

type TicketEmail struct {
	To             string
	CustomerName   string
	SalesEventName string
	StartsAt       time.Time
	Tickets        []IssuedTicket
}

type Sender interface {
	SendTickets(ctx context.Context, email TicketEmail) error
}

func NewSender(cfg SMTPConfig) Sender {
	if cfg.Host == "" {
		return LogSender{}
	}
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	if cfg.From == "" {
		cfg.From = cfg.Username
	}
	return SMTPSender{cfg: cfg}
}

type LogSender struct{}

func (LogSender) SendTickets(_ context.Context, email TicketEmail) error {
	slog.Info("ticket email skipped because smtp is not configured",
		"recipient", email.To,
		"sales_event_name", email.SalesEventName,
		"tickets", len(email.Tickets),
	)
	return nil
}

type SMTPSender struct {
	cfg SMTPConfig
}

func (s SMTPSender) SendTickets(ctx context.Context, email TicketEmail) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	message, err := buildTicketEmail(s.cfg.From, email)
	if err != nil {
		return err
	}

	addr := s.cfg.Host + ":" + s.cfg.Port
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	return smtp.SendMail(addr, auth, s.cfg.From, []string{email.To}, message)
}

func buildTicketEmail(from string, email TicketEmail) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	headers := textproto.MIMEHeader{}
	headers.Set("From", from)
	headers.Set("To", email.To)
	headers.Set("Subject", fmt.Sprintf("Your tickets for %s", email.SalesEventName))
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", `multipart/mixed; boundary="`+writer.Boundary()+`"`)

	for key, values := range headers {
		for _, value := range values {
			fmt.Fprintf(&body, "%s: %s\r\n", key, value)
		}
	}
	body.WriteString("\r\n")

	textHeaders := textproto.MIMEHeader{}
	textHeaders.Set("Content-Type", `text/plain; charset="utf-8"`)
	textPart, err := writer.CreatePart(textHeaders)
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf(
		"Hello %s,\n\nYour payment was confirmed for %s.\nEvent date: %s.\n\nYour QR code ticket(s) are attached to this email. Each QR code is unique and should be used by only one attendee.\n",
		email.CustomerName,
		email.SalesEventName,
		email.StartsAt.Format(time.RFC1123),
	)
	if _, err := textPart.Write([]byte(message)); err != nil {
		return nil, err
	}

	for _, ticket := range email.Tickets {
		partHeaders := textproto.MIMEHeader{}
		partHeaders.Set("Content-Type", "image/png")
		partHeaders.Set("Content-Transfer-Encoding", "base64")
		filename := safeAttachmentName(ticket.TicketName, ticket.ID)
		partHeaders.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

		part, err := writer.CreatePart(partHeaders)
		if err != nil {
			return nil, err
		}

		encoder := base64.NewEncoder(base64.StdEncoding, newLineWriter{w: part})
		if _, err := encoder.Write(ticket.QRCodePNG); err != nil {
			return nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return body.Bytes(), nil
}

func safeAttachmentName(ticketName string, ticketID string) string {
	name := strings.ToLower(ticketName)
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "\"", "", "'", "")
	name = replacer.Replace(name)
	if name == "" {
		name = "ticket"
	}
	return fmt.Sprintf("%s-%s.png", name, ticketID)
}

type newLineWriter struct {
	w interface {
		Write([]byte) (int, error)
	}
}

func (w newLineWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 76 {
		if _, err := w.w.Write(p[:76]); err != nil {
			return written, err
		}
		if _, err := w.w.Write([]byte("\r\n")); err != nil {
			return written, err
		}
		written += 76
		p = p[76:]
	}
	n, err := w.w.Write(p)
	return written + n, err
}
