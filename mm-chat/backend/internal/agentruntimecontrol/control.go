// Package agentruntimecontrol implements the G21.0 control-only Runner worker.
// It can probe, list and reconcile; it has no Run lease or launch surface.
package agentruntimecontrol

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

var (
	ErrInvalid     = errors.New("AGENT_CONTROL_INVALID")
	ErrUnavailable = errors.New("AGENT_CONTROL_UNAVAILABLE")
	fingerprintRE  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

var RequiredFeatures = []string{
	"cgroup_reap",
	"cgroup_v2",
	"network_none",
	"pidfd_kill",
	"readonly_rootfs",
	"rootless_userns",
	"seccomp",
	"snapshot_workspace",
	"subordinate_ids",
}

type Config struct {
	RunnerID     string
	PollInterval time.Duration
	BatchSize    int
}

type RPCClient interface {
	Call(context.Context, agentrunner.Request) (agentrunner.Response, error)
}

type Repository interface {
	RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error)
}

type ActivationGate interface {
	Verify(time.Time) error
}

type EvidenceGate struct{ Config agentactivation.Config }

func (gate EvidenceGate) Verify(now time.Time) error {
	decision, err := agentactivation.Verify(gate.Config, now)
	if err != nil || !decision.Ready {
		return ErrUnavailable
	}
	return nil
}

type Service struct {
	config     Config
	gate       ActivationGate
	repository Repository
	client     RPCClient
	now        func() time.Time
}

func NewService(config Config, gate ActivationGate, repository Repository, client RPCClient) (*Service, error) {
	if config.RunnerID == "" || config.PollInterval < 5*time.Second || config.PollInterval > 5*time.Minute ||
		config.BatchSize < 1 || config.BatchSize > 1000 || gate == nil || repository == nil || client == nil {
		return nil, ErrInvalid
	}
	return &Service{config: config, gate: gate, repository: repository, client: client, now: time.Now}, nil
}

func (service *Service) Run(ctx context.Context) error {
	if service == nil {
		return ErrInvalid
	}
	if err := service.Reconcile(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(service.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := service.Reconcile(ctx); err != nil {
				return err
			}
		}
	}
}

// Health performs no mutation. It validates the current activation, exact
// probe and equality between PostgreSQL-authorized and Runner inventories.
func (service *Service) Health(ctx context.Context) error {
	if service == nil {
		return ErrInvalid
	}
	now := service.now().UTC()
	probe, err := service.probe(ctx, now)
	if err != nil {
		return err
	}
	actual, err := service.list(ctx, now)
	if err != nil {
		return err
	}
	expected, err := service.expected(ctx, probe.ProbeFingerprint, now)
	if err != nil || !sameInventory(expected, actual) {
		return ErrUnavailable
	}
	return nil
}

func (service *Service) Reconcile(ctx context.Context) error {
	if service == nil {
		return ErrInvalid
	}
	now := service.now().UTC()
	probe, err := service.probe(ctx, now)
	if err != nil {
		return err
	}
	if _, err := service.list(ctx, now); err != nil {
		return err
	}
	expected, err := service.expected(ctx, probe.ProbeFingerprint, now)
	if err != nil {
		return err
	}
	request, err := agentrunner.NewRequest(agentrunner.MethodReconcile,
		agentrunner.ReconcileRequest{RunnerID: service.config.RunnerID, Expected: sortedInventory(expected)}, now)
	if err != nil {
		return ErrUnavailable
	}
	response, err := service.client.Call(ctx, request)
	if err != nil {
		return ErrUnavailable
	}
	result, ok := response.Body.(agentrunner.ReconcileResult)
	if !ok || result.RunnerID != service.config.RunnerID || result.Cleaned < 0 {
		return ErrUnavailable
	}
	actual, err := service.list(ctx, service.now().UTC())
	if err != nil || !sameInventory(expected, actual) {
		return ErrUnavailable
	}
	return nil
}

func (service *Service) probe(ctx context.Context, now time.Time) (agentrunner.ProbeResult, error) {
	if err := service.gate.Verify(now); err != nil {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	request, err := agentrunner.NewRequest(agentrunner.MethodProbe,
		agentrunner.ProbeRequest{RequiredFeatures: append([]string(nil), RequiredFeatures...)}, now)
	if err != nil {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	response, err := service.client.Call(ctx, request)
	if err != nil {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	result, ok := response.Body.(agentrunner.ProbeResult)
	if !ok || !result.Ready || result.Error != nil || !validFingerprint(result.ProbeFingerprint) ||
		!containsAll(result.Features, RequiredFeatures) {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	return result, nil
}

func (service *Service) list(ctx context.Context, now time.Time) (map[string]agentrunner.SandboxDescriptor, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: service.config.RunnerID}, now)
	if err != nil {
		return nil, ErrUnavailable
	}
	response, err := service.client.Call(ctx, request)
	if err != nil {
		return nil, ErrUnavailable
	}
	result, ok := response.Body.(agentrunner.ListResult)
	if !ok || result.RunnerID != service.config.RunnerID || len(result.Sandboxes) > service.config.BatchSize {
		return nil, ErrUnavailable
	}
	inventory := make(map[string]agentrunner.SandboxDescriptor, len(result.Sandboxes))
	for _, item := range result.Sandboxes {
		if item.Attempt.AttemptID == "" || inventory[item.Attempt.AttemptID].Attempt.AttemptID != "" {
			return nil, ErrUnavailable
		}
		inventory[item.Attempt.AttemptID] = item
	}
	return inventory, nil
}

func (service *Service) expected(ctx context.Context, probeFingerprint string, now time.Time) (map[string]agentrunner.SandboxDescriptor, error) {
	items, err := service.repository.RecoverySandboxes(ctx, service.config.BatchSize)
	if err != nil || len(items) > service.config.BatchSize {
		return nil, ErrUnavailable
	}
	inventory := make(map[string]agentrunner.SandboxDescriptor, len(items))
	for _, item := range items {
		if item.RunnerID != service.config.RunnerID || item.Attempt.AttemptID == "" ||
			inventory[item.Attempt.AttemptID].Attempt.AttemptID != "" {
			return nil, ErrUnavailable
		}
		if item.LeaseExpired || !now.Before(item.LeaseExpiresAt) || item.ProbeFingerprint != probeFingerprint {
			continue
		}
		inventory[item.Attempt.AttemptID] = agentrunner.SandboxDescriptor{
			SandboxID: item.SandboxID, Attempt: item.Attempt,
			SnapshotFingerprint: item.SnapshotFingerprint, SpecFingerprint: item.SpecFingerprint,
			ProbeFingerprint: item.ProbeFingerprint, State: item.SandboxState, UpdatedAt: now,
		}
	}
	return inventory, nil
}

func sameInventory(expected, actual map[string]agentrunner.SandboxDescriptor) bool {
	if len(expected) != len(actual) {
		return false
	}
	for attemptID, want := range expected {
		got, ok := actual[attemptID]
		if !ok || got.SandboxID != want.SandboxID || got.Attempt != want.Attempt ||
			got.SnapshotFingerprint != want.SnapshotFingerprint || got.SpecFingerprint != want.SpecFingerprint ||
			got.ProbeFingerprint != want.ProbeFingerprint || got.State != want.State {
			return false
		}
	}
	return true
}

func sortedInventory(values map[string]agentrunner.SandboxDescriptor) []agentrunner.SandboxDescriptor {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]agentrunner.SandboxDescriptor, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func containsAll(values, required []string) bool {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := set[value]; duplicate {
			return false
		}
		set[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func validFingerprint(value string) bool {
	return fingerprintRE.MatchString(value)
}
