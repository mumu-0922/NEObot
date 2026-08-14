package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
)

func loadWorkerConfig(lookup func(string) (string, bool)) (workerConfig, error) {
	var result workerConfig
	var err error
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{envCanaryEnabled, &result.canaryEnabled}, {envRuntimeEnabled, &result.runtimeEnabled},
		{envSchedulerEnabled, &result.schedulerEnabled}, {envSkillInstallEnabled, &result.skillInstallEnabled},
		{envLearningEnabled, &result.learningEnabled}, {envDelegationEnabled, &result.delegationEnabled},
		{envBrokerReadEnabled, &result.brokerReadEnabled}, {envBrokerWriteEnabled, &result.brokerWriteEnabled},
	} {
		*setting.target, err = boolSetting(lookup, setting.name, false)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if !result.canaryEnabled {
		return workerConfig{}, fmt.Errorf("%s must be true", envCanaryEnabled)
	}
	if result.runtimeEnabled || result.schedulerEnabled || result.skillInstallEnabled || result.learningEnabled ||
		result.delegationEnabled || result.brokerReadEnabled || result.brokerWriteEnabled {
		return workerConfig{}, errors.New("G21.2 permits only the synthetic Broker and Artifact canary")
	}
	for name, target := range map[string]*string{
		envDatabaseURL: &result.databaseURL, envRunnerURL: &result.runnerURL, envRunnerID: &result.runnerID,
		envRunnerServerName: &result.runnerServerName, envCallerIdentity: &result.callerIdentity,
		envRunnerClientCert: &result.runnerClientCert, envRunnerClientKey: &result.runnerClientKey,
		envRunnerServerCA: &result.runnerServerCA, envReleaseManifest: &result.releaseManifest,
		envProductionPolicy: &result.productionPolicy, envActivationRecord: &result.activationRecord,
		envCanaryPlan: &result.canaryPlan, envAuthorityPrivateKey: &result.authorityPrivateKey,
		envAuthorityPublicKey: &result.authorityPublicKey, envReleaseCommit: &result.releaseCommit,
		envRelayListen: &result.relayListen, envRelayEndpoint: &result.relayEndpoint,
		envRelayServerCert: &result.relayServerCert, envRelayServerKey: &result.relayServerKey,
		envRelayClientCA: &result.relayClientCA, envRunnerRelayIdentity: &result.runnerRelayIdentity,
		envQuarantineRoot: &result.quarantineRoot, envMCPRunnerURL: &result.mcpRunnerURL,
		envMCPRunnerTokenFile: &result.mcpRunnerTokenFile, "STORAGE_BACKEND": &result.storageBackend,
		"S3_ENDPOINT": &result.s3Endpoint, "S3_BUCKET": &result.s3Bucket, "S3_REGION": &result.s3Region,
		"S3_ACCESS_KEY_ID": &result.s3AccessKey, "S3_SECRET_ACCESS_KEY": &result.s3SecretKey,
	} {
		*target = env(lookup, name, "")
		if *target == "" {
			return workerConfig{}, fmt.Errorf("%s is required", name)
		}
	}
	if result.callerIdentity != agentactivation.BrokerCanaryCallerIdentity ||
		result.runnerRelayIdentity != agentactivation.RunnerRelayIdentity {
		return workerConfig{}, errors.New("G21.2 requires dedicated caller and relay identities")
	}
	for name, value := range map[string]string{
		envRunnerClientCert: result.runnerClientCert, envRunnerClientKey: result.runnerClientKey,
		envRunnerServerCA: result.runnerServerCA, envReleaseManifest: result.releaseManifest,
		envProductionPolicy: result.productionPolicy, envActivationRecord: result.activationRecord,
		envCanaryPlan: result.canaryPlan, envAuthorityPrivateKey: result.authorityPrivateKey,
		envAuthorityPublicKey: result.authorityPublicKey, envRelayServerCert: result.relayServerCert,
		envRelayServerKey: result.relayServerKey, envRelayClientCA: result.relayClientCA,
		envQuarantineRoot: result.quarantineRoot, envMCPRunnerTokenFile: result.mcpRunnerTokenFile,
	} {
		if !filepath.IsAbs(value) {
			return workerConfig{}, fmt.Errorf("%s must be absolute", name)
		}
	}
	if !privateAddress(result.relayListen) || !validRelayEndpoint(result.relayEndpoint, result.relayListen) {
		return workerConfig{}, errors.New("Broker canary relay must use the exact private literal HTTPS endpoint")
	}
	parsed, parseErr := url.Parse(result.databaseURL)
	password, hasPassword := "", false
	if parsed != nil && parsed.User != nil {
		password, hasPassword = parsed.User.Password()
	}
	if parseErr != nil || parsed == nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || !hasPassword || password == "" {
		return workerConfig{}, fmt.Errorf("%s must be a PostgreSQL URL", envDatabaseURL)
	}
	if len(result.releaseCommit) != 40 || strings.Trim(result.releaseCommit, "0123456789abcdef") != "" {
		return workerConfig{}, fmt.Errorf("%s must be a lowercase Git commit", envReleaseCommit)
	}
	result.storageBackend = strings.ToLower(result.storageBackend)
	if result.storageBackend != "minio" && result.storageBackend != "s3" {
		return workerConfig{}, errors.New("G21.2 requires the normal S3-compatible object store")
	}
	result.s3UseSSL, err = boolSetting(lookup, "S3_USE_SSL", false)
	if err != nil {
		return workerConfig{}, err
	}
	result.s3ForcePathStyle, err = boolSetting(lookup, "S3_FORCE_PATH_STYLE", result.storageBackend == "minio")
	if err != nil {
		return workerConfig{}, err
	}
	if autoCreate, autoErr := boolSetting(lookup, "S3_BUCKET_AUTO_CREATE", false); autoErr != nil || autoCreate {
		return workerConfig{}, errors.New("G21.2 object bucket auto-create must be false")
	}
	result.pollInterval, err = durationSetting(lookup, envPollInterval, 10*time.Second, time.Second, time.Minute)
	if err != nil {
		return workerConfig{}, err
	}
	result.rpcTimeout, err = durationSetting(lookup, envRPCTimeout, 10*time.Second, time.Second, 10*time.Second)
	if err != nil {
		return workerConfig{}, err
	}
	result.authorityTTL, err = durationSetting(lookup, envAuthorityTTL, 15*time.Second, 10*time.Second, 15*time.Second)
	if err != nil {
		return workerConfig{}, err
	}
	result.batchSize, err = intSetting(lookup, envBatchSize, 100, 1, 1000)
	if err != nil {
		return workerConfig{}, err
	}
	return result, nil
}

func verifyDatabaseRole(ctx context.Context, db *sql.DB) error {
	var accepted bool
	err := db.QueryRowContext(ctx, databaseRoleQuery).Scan(&accepted)
	if err != nil || !accepted {
		return errors.New("Broker canary database role rejected")
	}
	return nil
}

const databaseRoleQuery = `
WITH RECURSIVE current_login AS (
  SELECT oid,rolcanlogin,rolinherit,rolsuper,rolcreatedb,rolcreaterole,rolreplication,rolbypassrls
  FROM pg_roles WHERE rolname=current_user
), inherited(role_oid) AS (
  SELECT membership.roleid FROM pg_auth_members membership JOIN current_login ON current_login.oid=membership.member
  UNION
  SELECT membership.roleid FROM pg_auth_members membership JOIN inherited ON inherited.role_oid=membership.member
), inherited_names AS (
  SELECT role.rolname FROM inherited JOIN pg_roles role ON role.oid=inherited.role_oid
)
SELECT current_login.rolcanlogin AND current_login.rolinherit
  AND NOT current_login.rolsuper AND NOT current_login.rolcreatedb AND NOT current_login.rolcreaterole
  AND NOT current_login.rolreplication AND NOT current_login.rolbypassrls
  AND (SELECT array_agg(rolname ORDER BY rolname) FROM inherited_names)
      = ARRAY['agent_artifact_control','agent_effect_control','agent_orchestrator_runtime','agent_runner_control']::name[]
  AND NOT has_table_privilege(current_user,'agent_artifacts','INSERT,UPDATE,DELETE')
  AND NOT has_table_privilege(current_user,'agent_effect_intents','INSERT,UPDATE,DELETE')
  AND NOT has_table_privilege(current_user,'agent_attempts','INSERT,UPDATE,DELETE')
  AND has_function_privilege(current_user,
    'agent_artifact_authorize(text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)','EXECUTE')
  AND has_function_privilege(current_user,
    'agent_artifact_attach(text,text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)','EXECUTE')
FROM current_login`

func validRelayEndpoint(raw, listen string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Path != agentbrokerrelay.Path || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.User != nil || !privateAddress(parsed.Host) {
		return false
	}
	listenHost, listenPort, _ := net.SplitHostPort(listen)
	return parsed.Hostname() == strings.Trim(listenHost, "[]") && parsed.Port() == listenPort
}

func privateAddress(value string) bool {
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	ip := net.ParseIP(host)
	return err == nil && portNumber >= 1 && portNumber <= 65535 && ip != nil && !ip.IsUnspecified() &&
		(ip.IsLoopback() || ip.IsPrivate())
}

func readMCPRunnerToken(path string) (string, error) {
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = unix.Close(descriptor)
		return "", errors.New("MCP Runner token invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 32 || info.Size() > 4096 {
		return "", errors.New("MCP Runner token invalid")
	}
	payload, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(payload) > 4096 {
		clear(payload)
		return "", errors.New("MCP Runner token invalid")
	}
	token := strings.TrimSpace(string(payload))
	clear(payload)
	if len(token) < 32 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("MCP Runner token invalid")
	}
	return token, nil
}

func env(lookup func(string) (string, bool), name, fallback string) string {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func boolSetting(lookup func(string) (string, bool), name string, fallback bool) (bool, error) {
	value := env(lookup, name, strconv.FormatBool(fallback))
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}

func durationSetting(lookup func(string) (string, bool), name string, fallback, minimum,
	maximum time.Duration,
) (time.Duration, error) {
	parsed, err := time.ParseDuration(env(lookup, name, fallback.String()))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is out of range", name)
	}
	return parsed, nil
}

func intSetting(lookup func(string) (string, bool), name string, fallback, minimum, maximum int) (int, error) {
	parsed, err := strconv.Atoi(env(lookup, name, strconv.Itoa(fallback)))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s is out of range", name)
	}
	return parsed, nil
}
