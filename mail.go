package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// mailer sends a plain-text email (ADR 0008 Decision 7).
type mailer interface {
	send(to, subject, body string, headers map[string]string) error
}

// smtpMailer sends by SMTP, upgrading to TLS with STARTTLS, as Amazon SES
// and most providers offer on port 587.
type smtpMailer struct {
	addr, host, username, password, from string
}

// mailerFromEnv is the mailer SMTP_HOST, SMTP_PORT (default 587),
// SMTP_USERNAME, SMTP_PASSWORD and MAIL_FROM configure, or nil when they
// don't: then email isn't offered.
func mailerFromEnv() mailer {
	host, from := os.Getenv("SMTP_HOST"), os.Getenv("MAIL_FROM")
	if host == "" || from == "" {
		return nil
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return &smtpMailer{addr: host + ":" + port, host: host, username: os.Getenv("SMTP_USERNAME"), password: os.Getenv("SMTP_PASSWORD"), from: from}
}

func (m *smtpMailer) send(to, subject, body string, headers map[string]string) error {
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host) // only over TLS
	}
	return smtp.SendMail(m.addr, auth, m.from, []string{to}, emailMessage(m.from, to, subject, body, headers))
}

// emailMessage is an email as sent: headers, then the body as UTF-8 text with
// CRLF line endings.
func emailMessage(from, to, subject, body string, headers map[string]string) []byte {
	id := make([]byte, 12)
	rand.Read(id)
	domain := from[strings.LastIndex(from, "@")+1:]
	domain = strings.TrimSuffix(domain, ">")
	var b strings.Builder
	write := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	write("From", from)
	write("To", to)
	write("Subject", mime.QEncoding.Encode("utf-8", subject))
	write("Date", time.Now().Format(time.RFC1123Z))
	write("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	write("MIME-Version", "1.0")
	write("Content-Type", "text/plain; charset=utf-8")
	write("Content-Transfer-Encoding", "8bit")
	for k, v := range headers {
		write(k, v)
	}
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}
