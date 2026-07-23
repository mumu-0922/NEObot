package files

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	maxRemoteURLBytes       = 4096
	maxRemoteRedirects      = 3
	maxRemoteErrorBodyBytes = 4096
	remoteFetchTimeout      = 45 * time.Second
)

type fetchedRemoteFile struct {
	filename string
	mimeType string
	body     []byte
}

type remoteFileFetcher interface {
	Fetch(ctx context.Context, rawURL string, maxBytes int64) (fetchedRemoteFile, error)
}

type remoteHTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type secureRemoteFileFetcher struct {
	doer remoteHTTPDoer
}

func newSecureRemoteFileFetcher() remoteFileFetcher {
	return &secureRemoteFileFetcher{doer: newRemoteFileHTTPClient()}
}

func (f *secureRemoteFileFetcher) Fetch(
	ctx context.Context,
	rawURL string,
	maxBytes int64,
) (fetchedRemoteFile, error) {
	parsed, err := validateRemoteFileURL(rawURL)
	if err != nil {
		return fetchedRemoteFile{}, err
	}
	if maxBytes <= 0 {
		return fetchedRemoteFile{}, newValidationError(
			"FILE_TOO_LARGE",
			"remote file size limit is invalid",
		)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		parsed.String(),
		nil,
	)
	if err != nil {
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_URL_INVALID",
			"remote file URL is invalid",
		)
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "NeoChat-Remote-File/1")

	doer := f.doer
	if doer == nil {
		doer = newRemoteFileHTTPClient()
	}
	response, err := doer.Do(request)
	if err != nil {
		var validationError ValidationError
		if errors.As(err, &validationError) {
			return fetchedRemoteFile{}, validationError
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			return fetchedRemoteFile{}, newValidationError(
				"REMOTE_FETCH_TIMEOUT",
				"remote file download timed out",
			)
		}
		if ctx.Err() != nil {
			return fetchedRemoteFile{}, ctx.Err()
		}
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_FETCH_FAILED",
			"remote file could not be downloaded",
		)
	}
	if response == nil || response.Body == nil {
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_FETCH_FAILED",
			"remote file response is invalid",
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxRemoteErrorBodyBytes))
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_UPSTREAM_STATUS",
			"remote file server returned a non-success status",
		)
	}
	contentEncoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding")))
	if contentEncoding != "" && contentEncoding != "identity" {
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_CONTENT_ENCODING_UNSUPPORTED",
			"remote file response must use identity encoding",
		)
	}
	if response.ContentLength > maxBytes {
		return fetchedRemoteFile{}, newValidationError(
			"FILE_TOO_LARGE",
			"remote file exceeds upload limit",
		)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_FETCH_FAILED",
			"remote file could not be read",
		)
	}
	if int64(len(body)) > maxBytes {
		return fetchedRemoteFile{}, newValidationError(
			"FILE_TOO_LARGE",
			"remote file exceeds upload limit",
		)
	}
	if len(body) == 0 {
		return fetchedRemoteFile{}, newValidationError(
			"REMOTE_FILE_EMPTY",
			"remote file is empty",
		)
	}

	finalURL := parsed
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}
	return fetchedRemoteFile{
		filename: remoteFilename(response.Header, finalURL),
		mimeType: remoteMimeType(response.Header.Get("Content-Type"), body),
		body:     body,
	}, nil
}

func (s *Service) ImportRemote(
	ctx context.Context,
	input RemoteImportInput,
	maxBytes int64,
) (FileRecord, error) {
	if err := s.requireReady(); err != nil {
		return FileRecord{}, err
	}
	fetcher := s.remoteFetcher
	if fetcher == nil {
		fetcher = newSecureRemoteFileFetcher()
	}
	remote, err := fetcher.Fetch(ctx, input.URL, maxBytes)
	if err != nil {
		return FileRecord{}, err
	}
	return s.Upload(ctx, UploadInput{
		OriginalFilename: remote.filename,
		MimeType:         remote.mimeType,
		Size:             int64(len(remote.body)),
		Purpose:          input.Purpose,
		ConversationID:   input.ConversationID,
		WorkspaceID:      input.WorkspaceID,
		CollectionID:     input.CollectionID,
		ClientFileID:     input.ClientFileID,
		Body:             bytes.NewReader(remote.body),
	})
}

func validateRemoteFileURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > maxRemoteURLBytes {
		return nil, newValidationError("REMOTE_URL_INVALID", "remote file URL is invalid")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil {
		return nil, newValidationError(
			"REMOTE_URL_INVALID",
			"remote file URL must be a public HTTPS URL without credentials",
		)
	}
	if isLocalRemoteHostname(parsed.Hostname()) {
		return nil, newValidationError("REMOTE_URL_BLOCKED", "remote file URL is not public")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicRemoteIP(ip) {
		return nil, newValidationError("REMOTE_URL_BLOCKED", "remote file URL is not public")
	}
	return parsed, nil
}

func newRemoteFileHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext:           dialPublicRemoteAddress,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   remoteFetchTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRemoteRedirects {
				return newValidationError(
					"REMOTE_REDIRECT_LIMIT",
					"remote file exceeded redirect limit",
				)
			}
			if _, err := validateRemoteFileURL(request.URL.String()); err != nil {
				return err
			}
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			return nil
		},
	}
}

type remoteLookupFunc func(
	ctx context.Context,
	network string,
	host string,
) ([]netip.Addr, error)

func dialPublicRemoteAddress(ctx context.Context, network, address string) (net.Conn, error) {
	target, err := resolvePublicRemoteAddress(
		ctx,
		address,
		net.DefaultResolver.LookupNetIP,
	)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, target)
}

func resolvePublicRemoteAddress(
	ctx context.Context,
	address string,
	lookup remoteLookupFunc,
) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", newValidationError("REMOTE_URL_INVALID", "remote file address is invalid")
	}
	addresses, err := lookup(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return "", newValidationError("REMOTE_DNS_FAILED", "remote file host could not be resolved")
	}
	for _, address := range addresses {
		if !address.IsValid() || !isPublicRemoteIP(net.IP(address.AsSlice())) {
			return "", newValidationError("REMOTE_URL_BLOCKED", "remote file host is not public")
		}
	}
	return net.JoinHostPort(addresses[0].String(), port), nil
}

func isPublicRemoteIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() {
		return false
	}
	for _, network := range blockedRemoteNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

var blockedRemoteNetworks = mustParseRemoteNetworks(
	"100.64.0.0/10",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"240.0.0.0/4",
	"2001:db8::/32",
)

func mustParseRemoteNetworks(values ...string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			panic(fmt.Sprintf("parse blocked remote network %q: %v", value, err))
		}
		networks = append(networks, network)
	}
	return networks
}

func isLocalRemoteHostname(hostname string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	return hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") ||
		hostname == "local" || strings.HasSuffix(hostname, ".local")
}

func remoteFilename(header http.Header, finalURL *url.URL) string {
	if disposition := header.Get("Content-Disposition"); disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename := strings.TrimSpace(params["filename"]); filename != "" {
				return filename
			}
		}
	}
	if finalURL != nil {
		filename := path.Base(strings.TrimSuffix(finalURL.EscapedPath(), "/"))
		if decoded, err := url.PathUnescape(filename); err == nil {
			filename = decoded
		}
		if filename != "" && filename != "." && filename != "/" {
			return filename
		}
	}
	return "remote-file.bin"
}

func remoteMimeType(contentType string, body []byte) string {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "" {
		return mediaType
	}
	return http.DetectContentType(body)
}

func isTimeoutError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
