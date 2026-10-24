package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentStateMigration(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, previousStateName)
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pending.json"), []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := migrateStateDir(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(target, "pending.json"))
	if err != nil || string(b) != "pending" {
		t.Fatal("lost pending receipt", err)
	}
	if _, err = migrateStateDir(base); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = migrateStateDir(base); err == nil {
		t.Fatal("expected conflict")
	}
	if _, err = os.Stat(filepath.Join(target, "pending.json")); err != nil {
		t.Fatal(err)
	}
}
