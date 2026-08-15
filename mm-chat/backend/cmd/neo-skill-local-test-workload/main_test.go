package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatusFieldsRequireExactValues(t *testing.T) {
	status := []byte("Name:\tprobe\nCapEff:\t0000000000000000\nNoNewPrivs:\t1\n")
	if !statusFieldIsZero(status, "CapEff") || !statusFieldEquals(status, "NoNewPrivs", "1") {
		t.Fatal("valid status fields were rejected")
	}
	if statusFieldIsZero([]byte("CapEff:\t0000000000200000\n"), "CapEff") {
		t.Fatal("nonzero capabilities were accepted")
	}
	if statusFieldEquals(status, "NoNewPrivs", "0") {
		t.Fatal("NoNewPrivs drift was accepted")
	}
}

func TestAssertReadOnlyRejectsWritableDirectory(t *testing.T) {
	root := t.TempDir()
	if err := assertReadOnly(root, "probe"); err == nil {
		t.Fatal("writable directory was accepted as read-only")
	}
	if _, err := os.Stat(filepath.Join(root, "probe")); !os.IsNotExist(err) {
		t.Fatal("read-only negative probe left a file")
	}
}
