package auth

import (
	"crypto/tls"
	"errors"
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
	errSMTPRecoveryDialFailed       = errors.New("smtp recovery dial failed")
	errSMTPRecoveryDeadlineFailed   = errors.New("smtp recovery deadline failed")
	errSMTPRecoveryDeliveryFailed   = errors.New("smtp recovery delivery failed")
	errSMTPRecoveryInvalidRecipient = errors.New("smtp recovery recipient is invalid")
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

type smtpRecoverySettings struct {
	addr        string
	host        string
	username    string
	password    string
	fromAddress string
	fromHeader  string
	timeout     time.Duration
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
	request, err := buildSMTPRecoveryRequest(d.settings, message)
	if err != nil {
		return errSMTPRecoveryInvalidRecipient
	}

	conn, err := d.dial("tcp", d.settings.addr, d.settings.timeout)
	if err != nil {
		return errSMTPRecoveryDialFailed
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := conn.SetDeadline(time.Now().Add(d.settings.timeout)); err != nil {
		return errSMTPRecoveryDeadlineFailed
	}
	if err := d.send(conn, request); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	return nil
}

func validateSMTPRecoveryConfig(
	config SMTPRecoveryConfig,
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
	if config.QueueSize <= 0 || config.QueueSize > maxSMTPRecoveryQueueSize {
		return smtpRecoverySettings{}, errors.New(
			"smtp recovery queue size is invalid",
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

func buildSMTPRecoveryRequest(
	settings smtpRecoverySettings,
	message RecoveryMessage,
) (smtpRecoverySendRequest, error) {
	toAddress, err := parseSMTPMailbox(message.Email)
	if err != nil {
		return smtpRecoverySendRequest{}, err
	}
	toHeader := (&mail.Address{Address: toAddress}).String()

	var email strings.Builder
	writeSMTPHeader(&email, "From", settings.fromHeader)
	writeSMTPHeader(&email, "To", toHeader)
	writeSMTPHeader(&email, "Subject", recoveryEmailSubject)
	writeSMTPHeader(&email, "MIME-Version", "1.0")
	writeSMTPHeader(&email, "Content-Type", "text/plain; charset=UTF-8")
	writeSMTPHeader(&email, "Content-Transfer-Encoding", "8bit")
	email.WriteString("\r\n")
	email.WriteString("A password recovery was requested for your Neo Chat account.\r\n\r\n")
	email.WriteString("Copy this recovery token:\r\n")
	email.WriteString(message.Token)
	email.WriteString("\r\n\r\n")
	email.WriteString("This token expires at ")
	email.WriteString(message.ExpiresAt.UTC().Format(time.RFC3339))
	email.WriteString(".\r\n")

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
		return errSMTPRecoveryDeliveryFailed
	}

	client, err := smtp.NewClient(conn, request.host)
	if err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	defer func() {
		_ = client.Close()
	}()

	if err := client.StartTLS(request.tlsConfig.Clone()); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	if request.username != "" {
		auth := smtp.PlainAuth(
			"",
			request.username,
			request.password,
			request.host,
		)
		if err := client.Auth(auth); err != nil {
			return errSMTPRecoveryDeliveryFailed
		}
	}
	if err := client.Mail(request.fromAddress); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	if err := client.Rcpt(request.toAddress); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}

	writer, err := client.Data()
	if err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	if _, err := writer.Write(request.message); err != nil {
		_ = writer.Close()
		return errSMTPRecoveryDeliveryFailed
	}
	if err := writer.Close(); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	if err := client.Quit(); err != nil {
		return errSMTPRecoveryDeliveryFailed
	}
	return nil
}
