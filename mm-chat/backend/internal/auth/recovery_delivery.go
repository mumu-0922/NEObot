package auth

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	recoveryEmailSubject     = "Neo Chat password recovery"
	maxSMTPRecoveryQueueSize = 10_000
)

var (
	ErrSMTPDialFailed       = errors.New("smtp dial failed")
	ErrSMTPDeadlineFailed   = errors.New("smtp deadline failed")
	ErrSMTPDeliveryFailed   = errors.New("smtp delivery failed")
	ErrSMTPInvalidRecipient = errors.New("smtp recipient is invalid")
	ErrSMTPInvalidMessage   = errors.New("smtp message is invalid")

	errSMTPRecoveryDialFailed = fmt.Errorf(
		"smtp recovery dial failed: %w",
		ErrSMTPDialFailed,
	)
	errSMTPRecoveryDeadlineFailed = fmt.Errorf(
		"smtp recovery deadline failed: %w",
		ErrSMTPDeadlineFailed,
	)
	errSMTPRecoveryDeliveryFailed = fmt.Errorf(
		"smtp recovery delivery failed: %w",
		ErrSMTPDeliveryFailed,
	)
	errSMTPRecoveryInvalidRecipient = fmt.Errorf(
		"smtp recovery recipient is invalid: %w",
		ErrSMTPInvalidRecipient,
	)
)

type RecoveryMessage struct {
	Email     string
	Token     string
	ExpiresAt time.Time
}

type RecoveryDelivery interface {
	EnqueueRecovery(RecoveryMessage) bool
}

type SMTPRecoveryConfig struct {
	Addr      string
	Username  string
	Password  string
	From      string
	QueueSize int
	Timeout   time.Duration
}

type SMTPTransportConfig struct {
	Addr     string
	Username string
	Password string
	From     string
	Timeout  time.Duration
}

type smtpRecoverySettings struct {
	addr        string
	host        string
	username    string
	password    string
	fromAddress string
	fromHeader  string
	timeout     time.Duration
}

type SMTPPlaintextMessage struct {
	To        string
	Subject   string
	MessageID string
	TextBody  string
}

type smtpRecoverySendRequest struct {
	host        string
	username    string
	password    string
	fromAddress string
	toAddress   string
	tlsConfig   *tls.Config
	message     []byte
}

type smtpRecoveryDialFunc func(
	network string,
	address string,
	timeout time.Duration,
) (net.Conn, error)

type smtpRecoverySendFunc func(
	conn net.Conn,
	request smtpRecoverySendRequest,
) error

type SMTPSyncTransport struct {
	settings smtpRecoverySettings
	dial     smtpRecoveryDialFunc
	send     smtpRecoverySendFunc
}

type SMTPRecoveryDelivery struct {
	settings smtpRecoverySettings
	queue    chan RecoveryMessage
	dial     smtpRecoveryDialFunc
	send     smtpRecoverySendFunc

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	workers   sync.WaitGroup
}

var (
	_ RecoveryDelivery = (*SMTPRecoveryDelivery)(nil)
	_ io.Closer        = (*SMTPRecoveryDelivery)(nil)
)

func NewSMTPSyncTransport(
	config SMTPTransportConfig,
) (*SMTPSyncTransport, error) {
	settings, err := validateSMTPTransportConfig(config)
	if err != nil {
		return nil, err
	}

	return &SMTPSyncTransport{
		settings: settings,
		dial:     dialSMTPRecovery,
		send:     sendSMTPRecovery,
	}, nil
}

func (t *SMTPSyncTransport) Ready() bool {
	return t != nil &&
		t.settings.addr != "" &&
		t.settings.host != "" &&
		t.settings.fromAddress != "" &&
		t.settings.timeout > 0
}

func (t *SMTPSyncTransport) SendPlaintext(
	message SMTPPlaintextMessage,
) error {
	if !t.Ready() {
		return ErrSMTPDeliveryFailed
	}

	request, err := buildSMTPPlaintextRequest(t.settings, message)
	if err != nil {
		return err
	}

	conn, err := t.dial("tcp", t.settings.addr, t.settings.timeout)
	if err != nil {
		return ErrSMTPDialFailed
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := conn.SetDeadline(time.Now().Add(t.settings.timeout)); err != nil {
		return ErrSMTPDeadlineFailed
	}
	if err := t.send(conn, request); err != nil {
		return ErrSMTPDeliveryFailed
	}
	return nil
}

func NewSMTPRecoveryDelivery(
	config SMTPRecoveryConfig,
) (*SMTPRecoveryDelivery, error) {
	settings, err := validateSMTPRecoveryConfig(config)
	if err != nil {
		return nil, err
	}

	delivery := &SMTPRecoveryDelivery{
		settings: settings,
		queue:    make(chan RecoveryMessage, config.QueueSize),
		dial:     dialSMTPRecovery,
		send:     sendSMTPRecovery,
	}
	delivery.workers.Add(1)
	go delivery.run()

	return delivery, nil
}

// EnqueueRecovery adds a recovery message without waiting for SMTP I/O or
// queue capacity. A false result means the delivery has closed or its bounded
// queue is full.
func (d *SMTPRecoveryDelivery) EnqueueRecovery(message RecoveryMessage) bool {
	if d == nil {
		return false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return false
	}

	select {
	case d.queue <- message:
		return true
	default:
		return false
	}
}

// Close stops new enqueue operations, drains the bounded queue, and waits for
// the sole worker to exit. Repeated and concurrent calls are safe.
func (d *SMTPRecoveryDelivery) Close() error {
	if d == nil {
		return nil
	}

	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		close(d.queue)
		d.mu.Unlock()
	})
	d.workers.Wait()
	return nil
}

func (d *SMTPRecoveryDelivery) run() {
	defer d.workers.Done()
	for message := range d.queue {
		_ = d.deliverRecovery(message)
	}
}

func (d *SMTPRecoveryDelivery) deliverRecovery(message RecoveryMessage) error {
	mailMessage, err := buildSMTPRecoveryMessage(message)
	if err != nil {
		return errSMTPRecoveryInvalidRecipient
	}

	transport := &SMTPSyncTransport{
		settings: d.settings,
		dial:     d.dial,
		send:     d.send,
	}
	if err := transport.SendPlaintext(mailMessage); err != nil {
		switch {
		case errors.Is(err, ErrSMTPInvalidRecipient):
			return errSMTPRecoveryInvalidRecipient
		case errors.Is(err, ErrSMTPDialFailed):
			return errSMTPRecoveryDialFailed
		case errors.Is(err, ErrSMTPDeadlineFailed):
			return errSMTPRecoveryDeadlineFailed
		default:
			return errSMTPRecoveryDeliveryFailed
		}
	}
	return nil
}

func validateSMTPRecoveryConfig(
	config SMTPRecoveryConfig,
) (smtpRecoverySettings, error) {
	settings, err := validateSMTPTransportConfig(SMTPTransportConfig{
		Addr:     config.Addr,
		Username: config.Username,
		Password: config.Password,
		From:     config.From,
		Timeout:  config.Timeout,
	})
	if err != nil {
		return smtpRecoverySettings{}, err
	}
	if config.QueueSize <= 0 || config.QueueSize > maxSMTPRecoveryQueueSize {
		return smtpRecoverySettings{}, errors.New(
			"smtp recovery queue size is invalid",
		)
	}
	return settings, nil
}

func validateSMTPTransportConfig(
	config SMTPTransportConfig,
) (smtpRecoverySettings, error) {
	addr := strings.TrimSpace(config.Addr)
	if addr == "" || containsHeaderNewline(addr) {
		return smtpRecoverySettings{}, errors.New("smtp address is required")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil ||
		host == "" ||
		host != strings.TrimSpace(host) ||
		containsControl(host) {
		return smtpRecoverySettings{}, errors.New(
			"smtp address must contain a valid host and port",
		)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return smtpRecoverySettings{}, errors.New("smtp port is invalid")
	}

	from := strings.TrimSpace(config.From)
	fromAddress, err := parseSMTPMailbox(from)
	if err != nil {
		return smtpRecoverySettings{}, errors.New("smtp sender is invalid")
	}
	fromHeader := (&mail.Address{Address: fromAddress}).String()

	username := strings.TrimSpace(config.Username)
	if containsControl(username) {
		return smtpRecoverySettings{}, errors.New("smtp username is invalid")
	}
	if (username == "") != (config.Password == "") {
		return smtpRecoverySettings{}, errors.New(
			"smtp username and password must be configured together",
		)
	}
	if config.Timeout <= 0 {
		return smtpRecoverySettings{}, errors.New(
			"smtp recovery timeout must be positive",
		)
	}

	return smtpRecoverySettings{
		addr:        addr,
		host:        host,
		username:    username,
		password:    config.Password,
		fromAddress: fromAddress,
		fromHeader:  fromHeader,
		timeout:     config.Timeout,
	}, nil
}

func buildSMTPRecoveryMessage(
	message RecoveryMessage,
) (SMTPPlaintextMessage, error) {
	if _, err := parseSMTPMailbox(message.Email); err != nil {
		return SMTPPlaintextMessage{}, err
	}

	var body strings.Builder
	body.WriteString("A password recovery was requested for your Neo Chat account.\r\n\r\n")
	body.WriteString("Copy this recovery token:\r\n")
	body.WriteString(message.Token)
	body.WriteString("\r\n\r\n")
	body.WriteString("This token expires at ")
	body.WriteString(message.ExpiresAt.UTC().Format(time.RFC3339))
	body.WriteString(".\r\n")

	return SMTPPlaintextMessage{
		To:       message.Email,
		Subject:  recoveryEmailSubject,
		TextBody: body.String(),
	}, nil
}

func buildSMTPRecoveryRequest(
	settings smtpRecoverySettings,
	message RecoveryMessage,
) (smtpRecoverySendRequest, error) {
	mailMessage, err := buildSMTPRecoveryMessage(message)
	if err != nil {
		return smtpRecoverySendRequest{}, err
	}
	return buildSMTPPlaintextRequest(settings, mailMessage)
}

func buildSMTPPlaintextRequest(
	settings smtpRecoverySettings,
	message SMTPPlaintextMessage,
) (smtpRecoverySendRequest, error) {
	toAddress, err := parseSMTPMailbox(message.To)
	if err != nil {
		return smtpRecoverySendRequest{}, ErrSMTPInvalidRecipient
	}
	subject, err := normalizeSMTPHeaderValue(message.Subject, false)
	if err != nil {
		return smtpRecoverySendRequest{}, err
	}
	messageID, err := normalizeSMTPHeaderValue(message.MessageID, true)
	if err != nil {
		return smtpRecoverySendRequest{}, err
	}
	toHeader := (&mail.Address{Address: toAddress}).String()

	var email strings.Builder
	writeSMTPHeader(&email, "From", settings.fromHeader)
	writeSMTPHeader(&email, "To", toHeader)
	writeSMTPHeader(&email, "Subject", subject)
	if messageID != "" {
		writeSMTPHeader(&email, "Message-ID", messageID)
	}
	writeSMTPHeader(&email, "MIME-Version", "1.0")
	writeSMTPHeader(&email, "Content-Type", "text/plain; charset=UTF-8")
	writeSMTPHeader(&email, "Content-Transfer-Encoding", "8bit")
	email.WriteString("\r\n")
	writeSMTPBody(&email, message.TextBody)

	return smtpRecoverySendRequest{
		host:        settings.host,
		username:    settings.username,
		password:    settings.password,
		fromAddress: settings.fromAddress,
		toAddress:   toAddress,
		tlsConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: settings.host,
		},
		message: []byte(email.String()),
	}, nil
}

func normalizeSMTPHeaderValue(value string, allowEmpty bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" && allowEmpty {
		return "", nil
	}
	if value == "" || containsHeaderNewline(value) || containsControl(value) {
		return "", ErrSMTPInvalidMessage
	}
	if len(value) > 998 {
		return "", ErrSMTPInvalidMessage
	}
	return value, nil
}

func writeSMTPBody(email *strings.Builder, value string) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if index > 0 {
			email.WriteString("\r\n")
		}
		email.WriteString(line)
	}
}

func writeSMTPHeader(email *strings.Builder, name string, value string) {
	email.WriteString(name)
	email.WriteString(": ")
	email.WriteString(value)
	email.WriteString("\r\n")
}

func parseSMTPMailbox(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || containsHeaderNewline(value) || containsControl(value) {
		return "", errors.New("mailbox is invalid")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address == "" || containsControl(address.Address) {
		return "", errors.New("mailbox is invalid")
	}
	if len(address.Address) > 254 {
		return "", errors.New("mailbox is invalid")
	}
	return address.Address, nil
}

func containsHeaderNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func dialSMTPRecovery(
	network string,
	address string,
	timeout time.Duration,
) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	return dialer.Dial(network, address)
}

func sendSMTPRecovery(
	conn net.Conn,
	request smtpRecoverySendRequest,
) error {
	if request.tlsConfig == nil ||
		request.tlsConfig.MinVersion < tls.VersionTLS12 ||
		request.tlsConfig.ServerName != request.host ||
		request.tlsConfig.InsecureSkipVerify {
		return ErrSMTPDeliveryFailed
	}

	client, err := smtp.NewClient(conn, request.host)
	if err != nil {
		return ErrSMTPDeliveryFailed
	}
	defer func() {
		_ = client.Close()
	}()

	if err := client.StartTLS(request.tlsConfig.Clone()); err != nil {
		return ErrSMTPDeliveryFailed
	}
	if request.username != "" {
		auth := smtp.PlainAuth(
			"",
			request.username,
			request.password,
			request.host,
		)
		if err := client.Auth(auth); err != nil {
			return ErrSMTPDeliveryFailed
		}
	}
	if err := client.Mail(request.fromAddress); err != nil {
		return ErrSMTPDeliveryFailed
	}
	if err := client.Rcpt(request.toAddress); err != nil {
		return ErrSMTPDeliveryFailed
	}

	writer, err := client.Data()
	if err != nil {
		return ErrSMTPDeliveryFailed
	}
	if _, err := writer.Write(request.message); err != nil {
		_ = writer.Close()
		return ErrSMTPDeliveryFailed
	}
	if err := writer.Close(); err != nil {
		return ErrSMTPDeliveryFailed
	}
	if err := client.Quit(); err != nil {
		return ErrSMTPDeliveryFailed
	}
	return nil
}
