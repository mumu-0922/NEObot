package main

import (
	"strings"
	"testing"
)

func TestLoadWorkerConfigRequiresIndependentNarrowEnablement(t *testing.T) {
	values := map[string]string{
		envEnabled: "true", envDatabaseURL: "postgres://cron-worker",
		envPlanFile:         "/run/agent-cron-worker/plan.json",
		envProductionPolicy: "/run/agent-cron-worker/policy.json",
		envActivationRecord: "/run/agent-cron-worker/activation.json",
		envReleaseCommit:    strings.Repeat("a", 40),
		"AGENT_CRON_WORKER_CONTROL_ACTIVATION_FILE": "/run/agent-cron-worker/control.json",
		"AGENT_CRON_WORKER_ROOT_ACTIVATION_FILE":    "/run/agent-cron-worker/root.json",
		"AGENT_CRON_WORKER_BROKER_ACTIVATION_FILE":  "/run/agent-cron-worker/broker.json",
		"AGENT_CRON_WORKER_PROJECT_ACTIVATION_FILE": "/run/agent-cron-worker/project.json",
		"AGENT_CRON_WORKER_CHILD_ACTIVATION_FILE":   "/run/agent-cron-worker/child.json",
	}
	lookup := func(name string) (string, bool) { value, ok := values[name]; return value, ok }
	config, err := loadWorkerConfig(lookup)
	if err != nil || !config.enabled || config.runtimeEnabled || config.schedulerEnabled {
		t.Fatalf("loadWorkerConfig() = %#v, %v", config, err)
	}
	for _, flag := range []string{envRuntimeEnabled, envSchedulerEnabled, envLearningEnabled,
		envSkillInstallEnabled, envBrokerReadEnabled, envBrokerWriteEnabled, envDelegationEnabled} {
		t.Run(flag, func(t *testing.T) {
			candidate := map[string]string{}
			for key, value := range values {
				candidate[key] = value
			}
			candidate[flag] = "true"
			_, err := loadWorkerConfig(func(name string) (string, bool) {
				value, ok := candidate[name]
				return value, ok
			})
			if err == nil || !strings.Contains(err.Error(), "remain false") {
				t.Fatalf("loadWorkerConfig() error = %v", err)
			}
		})
	}
}
