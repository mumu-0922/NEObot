package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCaptureModeFlagsKeepsHoldoutExplicitAndExclusive(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "observations.json")
	sealPath := filepath.Join(directory, "holdout.seal.json")
	if err := validateCaptureModeFlags(
		true,
		"",
		"",
		"development,validation",
		"",
		0,
		"development.json",
		"validation.json",
		sealPath,
		outputPath,
	); err != nil {
		t.Fatalf("validateCaptureModeFlags() error = %v", err)
	}
	if err := validateCaptureModeFlags(
		false,
		"",
		"",
		"development,validation",
		"",
		0,
		"development.json",
		"",
		"",
		outputPath,
	); err == nil {
		t.Fatal("ordinary preflight accepted Holdout-only inputs")
	}
	if err := os.WriteFile(sealPath, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateCaptureModeFlags(
		true,
		"",
		"",
		"development,validation",
		"",
		0,
		"development.json",
		"validation.json",
		sealPath,
		outputPath,
	); err == nil {
		t.Fatal("existing Holdout seal was accepted")
	}
}

func TestValidateCaptureModeFlagsKeepsSupplementalModeExclusive(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "supplemental.json")
	suitePath := filepath.Join(directory, "suite.json")
	if err := validateCaptureModeFlags(
		false,
		suitePath,
		"",
		"development,validation",
		"",
		0,
		"",
		"",
		"",
		outputPath,
	); err != nil {
		t.Fatalf("validateCaptureModeFlags() error = %v", err)
	}
	if err := validateCaptureModeFlags(
		true,
		suitePath,
		"",
		"development,validation",
		"",
		0,
		"development.json",
		"validation.json",
		filepath.Join(directory, "holdout.seal.json"),
		outputPath,
	); err == nil {
		t.Fatal("supplemental mode was accepted together with Holdout mode")
	}
	if err := validateCaptureModeFlags(
		false,
		suitePath,
		"",
		"development",
		"case-1",
		1,
		"",
		"",
		"",
		outputPath,
	); err == nil {
		t.Fatal("supplemental mode accepted preflight case selection")
	}
	if err := os.WriteFile(outputPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateCaptureModeFlags(
		false,
		suitePath,
		"",
		"development,validation",
		"",
		0,
		"",
		"",
		"",
		outputPath,
	); err == nil {
		t.Fatal("supplemental mode accepted an existing output path")
	}
}

func TestValidateCaptureModeFlagsBindsLatencyDiagnosisToSupplementalMode(
	t *testing.T,
) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "latency.json")
	if err := validateCaptureModeFlags(
		false,
		"suite.json",
		"failed-supplemental.json",
		"development,validation",
		"",
		0,
		"",
		"",
		"",
		outputPath,
	); err != nil {
		t.Fatalf("validateCaptureModeFlags() error = %v", err)
	}
	if err := validateCaptureModeFlags(
		false,
		"",
		"failed-supplemental.json",
		"development,validation",
		"",
		0,
		"",
		"",
		"",
		outputPath,
	); err == nil {
		t.Fatal("latency diagnosis was accepted without a supplemental suite")
	}
}
