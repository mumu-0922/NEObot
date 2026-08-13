package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const credentialProbeTimeout = 5 * time.Second

// probeArtifactCredential performs only an exact manifest-declared GET. It
// deliberately does not call an arbitrary MCP Tool or trust Marketplace data.
func probeArtifactCredential(ctx context.Context, artifact Server, credential string) error {
	probe := artifact.CredentialProbe
	if probe == nil {
		return nil
	}
	if artifact.Transport != TransportStdio || artifact.Command == nil ||
		!containsString(artifact.Command.UserSecretEnv, probe.SecretField) {
		return ErrServerUnavailable
	}
	environment := map[string]string{}
	decoder := json.NewDecoder(strings.NewReader(credential))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&environment); err != nil {
		return ErrCredentialInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrCredentialInvalid
	}
	secret := environment[probe.SecretField]
	if secret == "" {
		return ErrCredentialRequired
	}
	endpoint, err := ValidateEndpoint(ctx, probe.EndpointURL, NetworkPolicy{RequireHTTPS: true})
	if err != nil {
		return ErrServerUnavailable
	}
	headers := make(http.Header)
	headers.Set(probe.HeaderName, probe.HeaderPrefix+secret)
	client := NewSafeHTTPClient(NetworkPolicy{RequireHTTPS: true}, endpoint, headers, credentialProbeTimeout)
	request, err := http.NewRequestWithContext(ctx, probe.Method, endpoint.String(), nil)
	if err != nil {
		return ErrServerUnavailable
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrServerUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return ErrCredentialInvalid
	default:
		return ErrServerUnavailable
	}
}
