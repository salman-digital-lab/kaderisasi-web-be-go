package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"net"
	"net/textproto"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestResetPasswordMatchesEdge(t *testing.T) {
	for _, url := range []string{"https://kaderisasi.example/reset?token=abc.def-_ghi", `https://x/?a=1&b=<"'>`, "https://x/?q=`="} {
		command := exec.Command("node", "../../scripts/edge-fixture.mjs")
		command.Stdin = strings.NewReader(url)
		want, err := command.Output()
		if err != nil {
			t.Fatalf("edge: %v", err)
		}
		if got := ResetPasswordHTML(url); got != string(want) {
			t.Fatalf("rendered email differs from Edge for %q", url)
		}
	}
}

func TestTemplateMatchesWebBe(t *testing.T) {
	original, err := os.ReadFile("../../../kaderisasi-web-be/resources/views/emails/reset_password.edge")
	if err != nil {
		t.Skip("kaderisasi-web-be checkout not present")
	}
	if string(original) != resetPasswordTemplate {
		t.Fatal("internal/mail/templates/reset_password.html is out of sync with web-be's reset_password.edge")
	}
}

// TestSMTPLoginAndMessage drives a minimal plain-text SMTP server on localhost.
func TestSMTPLoginAndMessage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan map[string]string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		text := textproto.NewConn(conn)
		got := map[string]string{}
		_ = text.PrintfLine("220 test ESMTP")
		for {
			line, err := text.ReadLine()
			if err != nil {
				return
			}
			verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
			switch verb {
			case "EHLO":
				_ = text.PrintfLine("250-test\r\n250 AUTH LOGIN")
			case "AUTH":
				_ = text.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
				user, _ := text.ReadLine()
				_ = text.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
				pass, _ := text.ReadLine()
				u, _ := base64.StdEncoding.DecodeString(user)
				p, _ := base64.StdEncoding.DecodeString(pass)
				got["auth"] = string(u) + ":" + string(p)
				_ = text.PrintfLine("235 ok")
			case "MAIL", "RCPT":
				got[verb] = line
				_ = text.PrintfLine("250 ok")
			case "DATA":
				_ = text.PrintfLine("354 go")
				data, _ := io.ReadAll(text.DotReader())
				got["data"] = string(data)
				_ = text.PrintfLine("250 queued")
			case "QUIT":
				_ = text.PrintfLine("221 bye")
				received <- got
				return
			default:
				_ = text.PrintfLine("250 ok")
			}
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	sender := SMTP{Host: "127.0.0.1", Port: port, Username: "mailer", Password: "secret"}
	err = sender.Send(context.Background(), Message{FromAddress: "noreply@example.test", FromName: "Kaderisasi Masjid Salman ITB", To: "member@example.test",
		Subject: "Reset kata sandi akun Kaderisasi Masjid Salman ITB", HTML: ResetPasswordHTML("https://example.test/reset?token=t")})
	if err != nil {
		t.Fatal(err)
	}
	got := <-received
	if got["auth"] != "mailer:secret" || got["MAIL"] != "MAIL FROM:<noreply@example.test>" || got["RCPT"] != "RCPT TO:<member@example.test>" {
		t.Fatalf("envelope: %+v", got)
	}
	headers, err := textproto.NewReader(bufio.NewReader(strings.NewReader(got["data"]))).ReadMIMEHeader()
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("From") != "Kaderisasi Masjid Salman ITB <noreply@example.test>" || headers.Get("Subject") != "Reset kata sandi akun Kaderisasi Masjid Salman ITB" || headers.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("headers: %v", headers)
	}
	body := got["data"][strings.Index(got["data"], "\r\n\r\n")+4:]
	decoded, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if !strings.Contains(string(decoded), `href="https://example.test/reset?token=t"`) {
		t.Fatal("reset link missing from body")
	}
}
