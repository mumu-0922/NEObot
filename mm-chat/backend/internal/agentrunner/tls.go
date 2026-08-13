package agentrunner

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"
)

type TLSFiles struct {
	CertificateFile string
	KeyFile         string
	ClientCAFile    string
}

func LoadServerTLS(files TLSFiles) (*tls.Config, error) {
	for _, path := range []string{files.CertificateFile, files.KeyFile, files.ClientCAFile} {
		if !strings.HasPrefix(strings.TrimSpace(path), "/") {
			return nil, errors.New("Runner TLS path is invalid")
		}
	}
	if err := securePrivateFile(files.KeyFile); err != nil {
		return nil, err
	}
	certificate, err := tls.LoadX509KeyPair(files.CertificateFile, files.KeyFile)
	if err != nil {
		return nil, errors.New("Runner TLS identity is unavailable")
	}
	caBytes, err := os.ReadFile(files.ClientCAFile)
	if err != nil || len(caBytes) > 1<<20 {
		return nil, errors.New("Runner client CA is unavailable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("Runner client CA is invalid")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: pool, SessionTicketsDisabled: true}, nil
}

func securePrivateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 1<<20 {
		return errors.New("Runner private key file is invalid")
	}
	return nil
}
