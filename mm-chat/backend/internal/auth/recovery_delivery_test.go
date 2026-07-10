package auth

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSMTPRecoveryDeliveryValidatesConfig(t *testing.T) {
	valid := SMTPRecoveryConfig{
		Addr:      "smtp.example.com:587",
		Username:  "mailer",
		Password:  "smtp-password-secret",
		From:      "Neo Chat <recovery@example.com>",
		QueueSize: 2,
		Timeout:   time.Second,
	}

	tests := []struct {
		name   string
		mutate func(*SMTPRecoveryConfig)
	}{
		{name: "missing address", mutate: func(c *SMTPRecoveryConfig) { c.Addr = "" }},
		{name: "address without port", mutate: func(c *SMTPRecoveryConfig) { c.Addr = "smtp.example.com" }},
		{name: "missing host", mutate: func(c *SMTPRecoveryConfig) { c.Addr = ":587" }},
		{name: "invalid port", mutate: func(c *SMTPRecoveryConfig) { c.Addr = "smtp.example.com:nope" }},
		{name: "missing sender", mutate: func(c *SMTPRecoveryConfig) { c.From = "" }},
		{name: "invalid sender", mutate: func(c *SMTPRecoveryConfig) { c.From = "not a mailbox" }},
		{name: "sender injection", mutate: func(c *SMTPRecoveryConfig) { c.From = "safe@example.com\r\nBcc: stolen@example.com" }},
		{name: "username without password", mutate: func(c *SMTPRecoveryConfig) { c.Password = "" }},
		{name: "password without username", mutate: func(c *SMTPRecoveryConfig) { c.Username = "" }},
		{name: "zero queue", mutate: func(c *SMTPRecoveryConfig) { c.QueueSize = 0 }},
		{name: "negative queue", mutate: func(c *SMTPRecoveryConfig) { c.QueueSize = -1 }},
		{name: "oversized queue", mutate: func(c *SMTPRecoveryConfig) { c.QueueSize = maxSMTPRecoveryQueueSize + 1 }},
		{name: "zero timeout", mutate: func(c *SMTPRecoveryConfig) { c.Timeout = 0 }},
		{name: "negative timeout", mutate: func(c *SMTPRecoveryConfig) { c.Timeout = -time.Second }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			delivery, err := NewSMTPRecoveryDelivery(config)
			if err == nil {
				delivery.Close()
				t.Fatal("NewSMTPRecoveryDelivery() error = nil")
			}
			if strings.Contains(err.Error(), valid.Password) {
				t.Fatal("NewSMTPRecoveryDelivery() exposed the SMTP password")
			}
		})
	}

	withoutAuth := valid
	withoutAuth.Username = ""
	withoutAuth.Password = ""
	delivery, err := NewSMTPRecoveryDelivery(withoutAuth)
	if err != nil {
		t.Fatalf("NewSMTPRecoveryDelivery() without auth error = %v", err)
	}
	delivery.Close()
}

func TestSMTPRecoveryDeliveryQueueIsBoundedAndNonBlocking(t *testing.T) {
	delivery := newTestSMTPRecoveryDelivery(t, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	delivery.send = func(net.Conn, smtpRecoverySendRequest) error {
		startedOnce.Do(func() { close(started) })
		<-release
		return nil
	}

	message := validRecoveryMessage()
	if !delivery.EnqueueRecovery(message) {
		t.Fatal("first EnqueueRecovery() = false")
	}
	waitForSignal(t, started, "SMTP worker start")
	if !delivery.EnqueueRecovery(message) || !delivery.EnqueueRecovery(message) {
		t.Fatal("EnqueueRecovery() did not fill available queue slots")
	}

	begin := time.Now()
	if delivery.EnqueueRecovery(message) {
		t.Fatal("EnqueueRecovery() on full queue = true")
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("EnqueueRecovery() blocked for %s", elapsed)
	}

	close(release)
	delivery.Close()
}

func TestSMTPRecoveryDeliveryCloseIsIdempotentAndWaits(t *testing.T) {
	delivery := newTestSMTPRecoveryDelivery(t, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	delivery.send = func(net.Conn, smtpRecoverySendRequest) error {
		close(started)
		<-release
		return nil
	}
	if !delivery.EnqueueRecovery(validRecoveryMessage()) {
		t.Fatal("EnqueueRecovery() = false")
	}
	waitForSignal(t, started, "SMTP worker start")

	closed := make(chan struct{})
	go func() {
		delivery.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close() returned before the worker exited")
	case <-time.After(25 * time.Millisecond):
	}

	close(release)
	waitForSignal(t, closed, "delivery close")
	delivery.Close()
	if delivery.EnqueueRecovery(validRecoveryMessage()) {
		t.Fatal("EnqueueRecovery() after Close() = true")
	}
}

func TestSMTPRecoveryMessageKeepsSecretsOutOfHeadersAndURLs(t *testing.T) {
	config := validSMTPRecoveryConfig(1)
	settings, err := validateSMTPRecoveryConfig(config)
	if err != nil {
		t.Fatalf("validateSMTPRecoveryConfig() error = %v", err)
	}
	token := "copy-only-token\r\nBcc: body-only@example.com"
	expiresAt := time.Date(2026, time.July, 10, 15, 30, 0, 0, time.UTC)
	request, err := buildSMTPRecoveryRequest(settings, RecoveryMessage{
		Email:     "Account Owner <owner@example.com>",
		Token:     token,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("buildSMTPRecoveryRequest() error = %v", err)
	}

	parts := strings.SplitN(string(request.message), "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("recovery message does not contain a header/body boundary")
	}
	headers, body := parts[0], parts[1]
	if strings.Contains(headers, token) || strings.Contains(headers, "body-only@example.com") {
		t.Fatal("recovery token content escaped into message headers")
	}
	if !strings.Contains(headers, "To: <owner@example.com>") {
		t.Fatalf("To header was not reduced to a safe mailbox: %q", headers)
	}
	if !strings.Contains(body, token) {
		t.Fatal("message body does not contain the copyable raw token")
	}
	if !strings.Contains(body, expiresAt.Format(time.RFC3339)) {
		t.Fatal("message body does not contain the expiration time")
	}
	for _, forbidden := range []string{
		"http://",
		"https://",
		"/recover",
		"?token=",
		"&token=",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("message body contains forbidden URL material %q", forbidden)
		}
	}
	if request.tlsConfig.MinVersion != tls.VersionTLS12 ||
		request.tlsConfig.ServerName != "smtp.example.com" ||
		request.tlsConfig.InsecureSkipVerify {
		t.Fatalf("TLS config is not strict: %#v", request.tlsConfig)
	}

	_, err = buildSMTPRecoveryRequest(settings, RecoveryMessage{
		Email: "owner@example.com\r\nBcc: stolen@example.com",
		Token: "secret",
	})
	if err == nil {
		t.Fatal("buildSMTPRecoveryRequest() accepted recipient header injection")
	}
}

func TestSMTPRecoveryDeliveryContinuesAfterSendErrorsWithoutSecretLeak(t *testing.T) {
	delivery := newTestSMTPRecoveryDelivery(t, 2)
	message := validRecoveryMessage()
	password := delivery.settings.password
	var calls atomic.Int32
	delivery.send = func(net.Conn, smtpRecoverySendRequest) error {
		calls.Add(1)
		return errors.New("upstream echoed " + password + " " + message.Token)
	}

	err := delivery.deliverRecovery(message)
	if !errors.Is(err, errSMTPRecoveryDeliveryFailed) {
		t.Fatalf("deliverRecovery() error = %v", err)
	}
	if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), message.Token) {
		t.Fatal("deliverRecovery() exposed an SMTP credential or recovery token")
	}

	if !delivery.EnqueueRecovery(message) || !delivery.EnqueueRecovery(message) {
		t.Fatal("EnqueueRecovery() = false")
	}
	delivery.Close()
	if got := calls.Load(); got != 3 {
		t.Fatalf("send calls = %d, want 3", got)
	}
}

func TestSMTPRecoveryDeliveryConcurrentClose(t *testing.T) {
	delivery := newTestSMTPRecoveryDelivery(t, 1)
	delivery.Close()

	var waiters sync.WaitGroup
	for range 8 {
		waiters.Add(1)
		go func() {
			defer waiters.Done()
			delivery.Close()
		}()
	}
	waiters.Wait()
}

func newTestSMTPRecoveryDelivery(
	t *testing.T,
	queueSize int,
) *SMTPRecoveryDelivery {
	t.Helper()
	delivery, err := NewSMTPRecoveryDelivery(validSMTPRecoveryConfig(queueSize))
	if err != nil {
		t.Fatalf("NewSMTPRecoveryDelivery() error = %v", err)
	}
	delivery.dial = func(string, string, time.Duration) (net.Conn, error) {
		return &smtpRecoveryTestConn{}, nil
	}
	return delivery
}

func validSMTPRecoveryConfig(queueSize int) SMTPRecoveryConfig {
	return SMTPRecoveryConfig{
		Addr:      "smtp.example.com:587",
		Username:  "mailer",
		Password:  "smtp-password-secret",
		From:      "Neo Chat <recovery@example.com>",
		QueueSize: queueSize,
		Timeout:   time.Second,
	}
}

func validRecoveryMessage() RecoveryMessage {
	return RecoveryMessage{
		Email:     "owner@example.com",
		Token:     "raw-recovery-token-secret",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

type smtpRecoveryTestConn struct{}

func (*smtpRecoveryTestConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (*smtpRecoveryTestConn) Write(data []byte) (int, error) {
	return len(data), nil
}

func (*smtpRecoveryTestConn) Close() error {
	return nil
}

func (*smtpRecoveryTestConn) LocalAddr() net.Addr {
	return smtpRecoveryTestAddr("local")
}

func (*smtpRecoveryTestConn) RemoteAddr() net.Addr {
	return smtpRecoveryTestAddr("remote")
}

func (*smtpRecoveryTestConn) SetDeadline(time.Time) error {
	return nil
}

func (*smtpRecoveryTestConn) SetReadDeadline(time.Time) error {
	return nil
}

func (*smtpRecoveryTestConn) SetWriteDeadline(time.Time) error {
	return nil
}

type smtpRecoveryTestAddr string

func (smtpRecoveryTestAddr) Network() string  { return "test" }
func (a smtpRecoveryTestAddr) String() string { return string(a) }
