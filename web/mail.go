package web

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// mailer sends a plain-text email (ADR 0008 Decision 7).
type mailer interface {
	send(to, subject, body string, headers map[string]string) error
}

// mailTimeout is the longest one email can take, from dialling to the
// server's last answer: a server that stops answering costs one try, not a
// worker for good.
const mailTimeout = 30 * time.Second

// smtpMailer sends by SMTP, upgrading to TLS with STARTTLS, as Amazon SES
// and most providers offer on port 587.
type smtpMailer struct {
	addr, host, username, password, from string
	timeout                              time.Duration // mailTimeout unless set
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
	timeout := m.timeout
	if timeout == 0 {
		timeout = mailTimeout
	}
	conn, err := net.DialTimeout("tcp", m.addr, timeout)
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(timeout))
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return err
		}
	}
	if m.username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("the mail server takes no login")
		}
		if err := c.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil { // only over TLS
			return err
		}
	}
	if err := c.Mail(envelope(m.from)); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(emailMessage(m.from, to, subject, body, headers)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// envelope is the address alone of MAIL_FROM, which may give a name too
// ("Competitions <noreply@example.com>").
func envelope(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		return a.Address
	}
	return from
}

// emailMessage is an email as sent: headers, a name in From or the subject
// encoded as RFC 2047 asks, then the body as UTF-8 text, quoted-printable,
// so no line is longer than SMTP allows however long a paragraph is.
func emailMessage(from, to, subject, body string, headers map[string]string) []byte {
	id := make([]byte, 12)
	rand.Read(id)
	if a, err := mail.ParseAddress(from); err == nil {
		from = a.String()
	}
	address := envelope(from)
	domain := address[strings.LastIndex(address, "@")+1:]
	var b strings.Builder
	write := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	write("From", from)
	write("To", to)
	write("Subject", mime.QEncoding.Encode("utf-8", subject))
	write("Date", time.Now().Format(time.RFC1123Z))
	write("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	write("MIME-Version", "1.0")
	write("Content-Type", "text/plain; charset=utf-8")
	write("Content-Transfer-Encoding", "quoted-printable")
	for k, v := range headers {
		write(k, v)
	}
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	qp.Write([]byte(strings.ReplaceAll(body, "\r\n", "\n")))
	qp.Close()
	return []byte(b.String())
}
