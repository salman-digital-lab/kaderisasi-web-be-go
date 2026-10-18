// Package mail sends web-be's transactional email over SMTP the way
// @adonisjs/mail's SMTP transport (nodemailer) is configured in
// kaderisasi-web-be/config/mail.ts: AUTH LOGIN, implicit TLS on port 465,
// STARTTLS when the server offers it on other ports.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
)

//go:embed templates/reset_password.html
var resetPasswordTemplate string

// ResetPasswordHTML renders resources/views/emails/reset_password.edge. Edge's
// `{{ }}` HTML-escapes the value; the rest of the template is copied verbatim.
func ResetPasswordHTML(resetURL string) string {
	return strings.Replace(resetPasswordTemplate, "{{ resetUrl }}", escapeHTML(resetURL), 1)
}

// escapeHTML matches Edge 6 (he.escape): & < > ' " ` as &amp; &lt; &gt; &#x27; &quot; &#x60;.
func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "'", "&#x27;", `"`, "&quot;", "`", "&#x60;").Replace(s)
}

type Message struct {
	FromAddress, FromName, To, Subject, HTML string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}

type SMTP struct {
	Host, Port, Username, Password string
	// TLSConfig overrides certificate checks (tests only).
	TLSConfig *tls.Config
}

func (s SMTP) Send(ctx context.Context, m Message) error {
	if _, err := mail.ParseAddress(m.To); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	address := net.JoinHostPort(s.Host, s.Port)
	tlsConfig := s.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	var err error
	if s.Port == "465" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}
	if err := client.Hello(hostname); err != nil {
		return err
	}
	if s.Port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}
	if s.Username != "" {
		if err := client.Auth(loginAuth{s.Username, s.Password}); err != nil {
			return err
		}
	}
	if err := client.Mail(m.FromAddress); err != nil {
		return err
	}
	if err := client.Rcpt(m.To); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(build(m, hostname, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// loginAuth is SMTP AUTH LOGIN, which net/smtp does not provide.
type loginAuth struct{ username, password string }

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLocal(server.Name) {
		return "", nil, errors.New("refusing to send SMTP credentials without TLS")
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected AUTH LOGIN challenge %q", fromServer)
}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

var plainPhrase = regexp.MustCompile(`^[A-Za-z0-9 ]+$`)

func build(m Message, hostname string, now time.Time) []byte {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	from := (&mail.Address{Name: m.FromName, Address: m.FromAddress}).String()
	if plainPhrase.MatchString(m.FromName) {
		// nodemailer leaves a plain word phrase unquoted.
		from = m.FromName + " <" + m.FromAddress + ">"
	}
	var b bytes.Buffer
	for _, header := range [][2]string{
		{"From", from},
		{"To", m.To},
		{"Subject", mime.QEncoding.Encode("utf-8", m.Subject)},
		{"Message-ID", "<" + hex.EncodeToString(id) + "@" + hostname + ">"},
		{"Date", now.Format(time.RFC1123Z)},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/html; charset=utf-8"},
		{"Content-Transfer-Encoding", "quoted-printable"},
	} {
		b.WriteString(header[0] + ": " + header[1] + "\r\n")
	}
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(m.HTML, "\r\n", "\n"), "\n", "\r\n")))
	_ = qp.Close()
	return b.Bytes()
}
