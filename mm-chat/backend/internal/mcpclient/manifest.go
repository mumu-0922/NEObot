package mcpclient

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	manifestVersion       = 1
	maxManifestBytes      = 1 << 20
	maxManifestServers    = 100
	maxManifestArgs       = 64
	maxManifestEnv        = 64
	maxManifestString     = 4096
	defaultRunnerIdle     = 15 * time.Minute
	defaultRunnerLifetime = 24 * time.Hour
)

var (
	manifestIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	environmentPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

	//go:embed catalog.json
	embeddedCatalog []byte
)

type manifestDocument struct {
	Version int              `json:"version"`
	Servers []manifestServer `json:"servers"`
}

type manifestServer struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Transport   string            `json:"transport"`
	EndpointURL string            `json:"endpointUrl,omitempty"`
	Command     *manifestCommand  `json:"command,omitempty"`
	Auth        *manifestAuth     `json:"auth,omitempty"`
	Grants      []manifestGrant   `json:"grants,omitempty"`
	ToolPolicy  map[string]string `json:"toolPolicy,omitempty"`
}

type manifestCommand struct {
	Argv             []string      `json:"argv"`
	Env              []manifestEnv `json:"env,omitempty"`
	WorkingDirectory string        `json:"workingDirectory,omitempty"`
	IdleTimeout      string        `json:"idleTimeout,omitempty"`
	MaxLifetime      string        `json:"maxLifetime,omitempty"`
}

type manifestEnv struct {
	Name       string `json:"name"`
	Value      string `json:"value,omitempty"`
	SecretFile string `json:"secretFile,omitempty"`
	EnvRef     string `json:"envRef,omitempty"`
}

type manifestAuth struct {
	Type       string   `json:"type"`
	HeaderName string   `json:"headerName,omitempty"`
	SecretFile string   `json:"secretFile,omitempty"`
	EnvRef     string   `json:"envRef,omitempty"`
	ClientID   string   `json:"clientId,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

type manifestGrant struct {
	ScopeType     string `json:"scopeType"`
	ScopeID       string `json:"scopeId,omitempty"`
	DefaultEnable bool   `json:"defaultEnabled,omitempty"`
}

type Catalog struct {
	Version int      `json:"version"`
	Servers []Server `json:"servers"`
}

func LoadManifest(path string, lookupEnv func(string) (string, bool)) ([]Server, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open manifest", ErrManifestInvalid)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil || len(data) > maxManifestBytes {
		return nil, fmt.Errorf("%w: read manifest", ErrManifestInvalid)
	}
	return parseManifest(data, lookupEnv)
}

func LoadCatalog() (Catalog, error) {
	var raw struct {
		Version int `json:"version"`
		Servers []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			EndpointURL string `json:"endpointUrl"`
		} `json:"servers"`
	}
	if err := decodeStrictJSON(embeddedCatalog, &raw); err != nil || raw.Version != manifestVersion {
		return Catalog{}, fmt.Errorf("%w: embedded catalog", ErrManifestInvalid)
	}
	catalog := Catalog{Version: raw.Version, Servers: make([]Server, 0, len(raw.Servers))}
	seen := map[string]struct{}{}
	for _, entry := range raw.Servers {
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Name = strings.TrimSpace(entry.Name)
		if !manifestIDPattern.MatchString(entry.ID) || entry.Name == "" {
			return Catalog{}, fmt.Errorf("%w: embedded catalog entry", ErrManifestInvalid)
		}
		if _, ok := seen[entry.ID]; ok {
			return Catalog{}, fmt.Errorf("%w: duplicate catalog id", ErrManifestInvalid)
		}
		seen[entry.ID] = struct{}{}
		catalog.Servers = append(catalog.Servers, Server{
			Ref:         ServerRef{Source: "catalog", ID: entry.ID},
			Name:        entry.Name,
			Description: strings.TrimSpace(entry.Description),
			Transport:   TransportStreamableHTTP,
			EndpointURL: strings.TrimSpace(entry.EndpointURL),
			AuthType:    AuthNone,
			Status:      ServerStatusDraft,
		})
	}
	return catalog, nil
}

func parseManifest(data []byte, lookupEnv func(string) (string, bool)) ([]Server, error) {
	var document manifestDocument
	if err := decodeStrictJSON(data, &document); err != nil {
		return nil, fmt.Errorf("%w: invalid json", ErrManifestInvalid)
	}
	if document.Version != manifestVersion || len(document.Servers) > maxManifestServers {
		return nil, fmt.Errorf("%w: unsupported version or server count", ErrManifestInvalid)
	}
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	seen := make(map[string]struct{}, len(document.Servers))
	servers := make([]Server, 0, len(document.Servers))
	for _, raw := range document.Servers {
		server, err := normalizeManifestServer(raw, lookupEnv)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[server.Ref.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate server id", ErrManifestInvalid)
		}
		seen[server.Ref.ID] = struct{}{}
		servers = append(servers, server)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Ref.ID < servers[j].Ref.ID })
	return servers, nil
}

func normalizeManifestServer(raw manifestServer, lookupEnv func(string) (string, bool)) (Server, error) {
	raw.ID = strings.TrimSpace(raw.ID)
	raw.Name = strings.TrimSpace(raw.Name)
	raw.Description = strings.TrimSpace(raw.Description)
	raw.Transport = strings.TrimSpace(raw.Transport)
	if !manifestIDPattern.MatchString(raw.ID) || raw.Name == "" || len(raw.Name) > 256 ||
		len(raw.Description) > 2048 {
		return Server{}, fmt.Errorf("%w: invalid server identity", ErrManifestInvalid)
	}
	server := Server{
		Ref:         ServerRef{Source: SourceManifest, ID: raw.ID},
		Name:        raw.Name,
		Description: raw.Description,
		Transport:   raw.Transport,
		AuthType:    AuthNone,
		Status:      ServerStatusReady,
		Metadata:    map[string]any{"toolPolicy": cloneStringMap(raw.ToolPolicy)},
	}
	switch raw.Transport {
	case TransportStreamableHTTP:
		server.EndpointURL = strings.TrimSpace(raw.EndpointURL)
		if server.EndpointURL == "" || raw.Command != nil {
			return Server{}, fmt.Errorf("%w: remote server shape", ErrManifestInvalid)
		}
		if _, err := parseEndpoint(server.EndpointURL, false); err != nil {
			return Server{}, fmt.Errorf("%w: remote endpoint", ErrManifestInvalid)
		}
	case TransportStdio:
		if strings.TrimSpace(raw.EndpointURL) != "" || raw.Command == nil {
			return Server{}, fmt.Errorf("%w: stdio server shape", ErrManifestInvalid)
		}
		command, err := resolveManifestCommand(*raw.Command, lookupEnv)
		if err != nil {
			return Server{}, err
		}
		server.Command = &command
	default:
		return Server{}, fmt.Errorf("%w: unsupported transport", ErrManifestInvalid)
	}
	if raw.Auth != nil {
		if err := applyManifestAuth(&server, *raw.Auth, lookupEnv); err != nil {
			return Server{}, err
		}
	}
	for _, rawGrant := range raw.Grants {
		grant := Grant{
			ScopeType:     strings.TrimSpace(rawGrant.ScopeType),
			ScopeID:       strings.TrimSpace(rawGrant.ScopeID),
			DefaultEnable: rawGrant.DefaultEnable,
		}
		switch grant.ScopeType {
		case "global":
			if grant.ScopeID != "" {
				return Server{}, fmt.Errorf("%w: global grant scope", ErrManifestInvalid)
			}
		case "team", "workspace":
			if !validUUID(grant.ScopeID) {
				return Server{}, fmt.Errorf("%w: scoped grant id", ErrManifestInvalid)
			}
		default:
			return Server{}, fmt.Errorf("%w: grant scope", ErrManifestInvalid)
		}
		server.Grants = append(server.Grants, grant)
	}
	if len(server.Grants) == 0 {
		server.Grants = []Grant{{ScopeType: "global"}}
	}
	return server, nil
}

func resolveManifestCommand(raw manifestCommand, lookupEnv func(string) (string, bool)) (Command, error) {
	if len(raw.Argv) == 0 || len(raw.Argv) > maxManifestArgs || len(raw.Env) > maxManifestEnv {
		return Command{}, fmt.Errorf("%w: command limits", ErrManifestInvalid)
	}
	argv := make([]string, len(raw.Argv))
	for index, value := range raw.Argv {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maxManifestString || strings.ContainsRune(value, '\x00') {
			return Command{}, fmt.Errorf("%w: command argument", ErrManifestInvalid)
		}
		argv[index] = value
	}
	if !filepath.IsAbs(argv[0]) || isShellExecutable(argv[0]) {
		return Command{}, fmt.Errorf("%w: command executable", ErrManifestInvalid)
	}
	workingDirectory := strings.TrimSpace(raw.WorkingDirectory)
	if workingDirectory != "" {
		return Command{}, fmt.Errorf("%w: working directory", ErrManifestInvalid)
	}
	environment := make(map[string]string, len(raw.Env))
	for _, entry := range raw.Env {
		name := strings.TrimSpace(entry.Name)
		if !environmentPattern.MatchString(name) || !safeCommandEnvironmentName(name) {
			return Command{}, fmt.Errorf("%w: environment name", ErrManifestInvalid)
		}
		if _, exists := environment[name]; exists {
			return Command{}, fmt.Errorf("%w: duplicate environment name", ErrManifestInvalid)
		}
		value, err := resolveManifestValue(entry.Value, entry.SecretFile, entry.EnvRef, lookupEnv)
		if err != nil {
			return Command{}, err
		}
		environment[name] = value
	}
	idle, err := parseBoundedDuration(raw.IdleTimeout, defaultRunnerIdle, time.Minute, time.Hour)
	if err != nil {
		return Command{}, err
	}
	lifetime, err := parseBoundedDuration(raw.MaxLifetime, defaultRunnerLifetime, time.Minute, 24*time.Hour)
	if err != nil || lifetime < idle {
		return Command{}, fmt.Errorf("%w: command lifetime", ErrManifestInvalid)
	}
	return Command{
		Argv:             argv,
		Env:              environment,
		WorkingDirectory: "",
		IdleTimeout:      idle,
		MaxLifetime:      lifetime,
	}, nil
}

func safeCommandEnvironmentName(name string) bool {
	switch name {
	case "PATH", "HOME", "TMPDIR", "TMP", "TEMP", "LD_PRELOAD", "LD_LIBRARY_PATH",
		"NODE_OPTIONS", "PYTHONPATH", "PYTHONHOME", "RUBYOPT", "GEM_HOME",
		"PERL5OPT", "BASH_ENV", "ENV", "IFS":
		return false
	default:
		return !strings.HasPrefix(name, "DYLD_")
	}
}

func isShellExecutable(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "sh", "bash", "dash", "zsh", "ksh", "fish", "pwsh", "powershell", "powershell.exe", "cmd", "cmd.exe":
		return true
	default:
		return false
	}
}

func applyManifestAuth(server *Server, raw manifestAuth, lookupEnv func(string) (string, bool)) error {
	raw.Type = strings.TrimSpace(raw.Type)
	switch raw.Type {
	case "", AuthNone:
		server.AuthType = AuthNone
		return nil
	case AuthHeader:
		name := strings.TrimSpace(raw.HeaderName)
		if !validHeaderName(name) || strings.EqualFold(name, "host") || strings.EqualFold(name, "cookie") {
			return fmt.Errorf("%w: header name", ErrManifestInvalid)
		}
		value, err := resolveManifestValue("", raw.SecretFile, raw.EnvRef, lookupEnv)
		if err != nil || value == "" {
			return fmt.Errorf("%w: header secret", ErrManifestInvalid)
		}
		server.AuthType = AuthHeader
		server.HeaderAuth = &HeaderAuth{Name: name, EncryptedSecret: value, SecretFile: raw.SecretFile, EnvRef: raw.EnvRef}
		return nil
	case AuthOAuth:
		clientID := strings.TrimSpace(raw.ClientID)
		if clientID == "" || len(clientID) > 2048 {
			return fmt.Errorf("%w: oauth client", ErrManifestInvalid)
		}
		clientSecret := ""
		if strings.TrimSpace(raw.SecretFile) != "" || strings.TrimSpace(raw.EnvRef) != "" {
			var err error
			clientSecret, err = resolveManifestValue("", raw.SecretFile, raw.EnvRef, lookupEnv)
			if err != nil {
				return fmt.Errorf("%w: oauth client secret", ErrManifestInvalid)
			}
		}
		server.AuthType = AuthOAuth
		server.OAuthClient = &OAuthClient{
			ClientID: clientID, ClientSecret: clientSecret,
			Scopes: normalizeStrings(raw.Scopes, 32, 256),
		}
		return nil
	default:
		return fmt.Errorf("%w: auth type", ErrManifestInvalid)
	}
}

func resolveManifestValue(literal, secretFile, envRef string, lookupEnv func(string) (string, bool)) (string, error) {
	literal = strings.TrimSpace(literal)
	secretFile = strings.TrimSpace(secretFile)
	envRef = strings.TrimSpace(envRef)
	configured := 0
	for _, value := range []string{literal, secretFile, envRef} {
		if value != "" {
			configured++
		}
	}
	if configured != 1 {
		return "", fmt.Errorf("%w: value source", ErrManifestInvalid)
	}
	if literal != "" {
		if len(literal) > maxManifestString || strings.ContainsRune(literal, '\x00') {
			return "", fmt.Errorf("%w: literal value", ErrManifestInvalid)
		}
		return literal, nil
	}
	if secretFile != "" {
		clean := filepath.Clean(secretFile)
		if !filepath.IsAbs(clean) || !strings.HasPrefix(clean, "/run/secrets/") {
			return "", fmt.Errorf("%w: secret file", ErrManifestInvalid)
		}
		data, err := os.ReadFile(clean)
		if err != nil || len(data) == 0 || len(data) > 64*1024 {
			return "", fmt.Errorf("%w: secret file", ErrManifestInvalid)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if !environmentPattern.MatchString(envRef) || !strings.HasPrefix(envRef, "MCP_SECRET_") {
		return "", fmt.Errorf("%w: environment reference", ErrManifestInvalid)
	}
	value, ok := lookupEnv(envRef)
	value = strings.TrimSpace(value)
	if !ok || value == "" || len(value) > 64*1024 {
		return "", fmt.Errorf("%w: environment reference", ErrManifestInvalid)
	}
	return value, nil
}

func parseBoundedDuration(raw string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: duration", ErrManifestInvalid)
	}
	return value, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing json")
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return output
}
