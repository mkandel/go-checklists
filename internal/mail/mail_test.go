package mail

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// startStubSMTP starts a minimal SMTP server on 127.0.0.1 that accepts
// exactly one connection, speaks just enough of the protocol (EHLO/AUTH
// PLAIN/MAIL FROM/RCPT TO/DATA/.) to satisfy net/smtp.SendMail, and reports
// whether it saw the expected sender/recipient/body. It returns the port to
// dial and a channel that receives the received message body once the
// session completes.
func startStubSMTP(t *testing.T) (port int, received <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ch := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		r := bufio.NewReader(conn)
		w := conn

		writeLine := func(s string) { w.Write([]byte(s + "\r\n")) }
		writeLine("220 stub.local ESMTP")

		var bodyLines []string
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")

			if inData {
				if line == "." {
					inData = false
					writeLine("250 OK")
					ch <- strings.Join(bodyLines, "\n")
					continue
				}
				bodyLines = append(bodyLines, line)
				continue
			}

			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"):
				writeLine("250-stub.local")
				writeLine("250 AUTH PLAIN")
			case strings.HasPrefix(upper, "AUTH PLAIN"):
				writeLine("235 OK")
			case strings.HasPrefix(upper, "MAIL FROM"):
				writeLine("250 OK")
			case strings.HasPrefix(upper, "RCPT TO"):
				writeLine("250 OK")
			case upper == "DATA":
				inData = true
				writeLine("354 Start mail input")
			case upper == "QUIT":
				writeLine("221 Bye")
				return
			default:
				writeLine("500 unrecognized command")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port, ch
}

func TestSend(t *testing.T) {
	port, received := startStubSMTP(t)

	cfg := SMTPConfig{
		Host:        "127.0.0.1",
		Port:        port,
		Username:    "user",
		Password:    "pass",
		FromAddress: "notifications@example.com",
	}
	msg := Message{
		To:      "recipient@example.com",
		Subject: "Test subject",
		Body:    "Test body.",
	}

	if err := Send(cfg, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	body := <-received
	if !strings.Contains(body, "Subject: Test subject") {
		t.Errorf("body missing subject header: %q", body)
	}
	if !strings.Contains(body, "Test body.") {
		t.Errorf("body missing message text: %q", body)
	}
	if !strings.Contains(body, "To: recipient@example.com") {
		t.Errorf("body missing To header: %q", body)
	}
}
