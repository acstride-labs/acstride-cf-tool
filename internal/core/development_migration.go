package core

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// Historical names are used only to migrate development installs, never for new writes.
const previousStateName = "acstride-cf-tool"

func migrateStateDir(base string) (string, error) {
	target := filepath.Join(base, "codeforces-client")
	source := filepath.Join(base, previousStateName)
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return target, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", Fail("unsafe_path", "Previous receipt state must be a real directory.")
	}
	if _, err = os.Lstat(target); err == nil {
		return "", Fail("migration_conflict", "Both previous and current receipt directories exist. Reconcile them before submitting; no receipts were overwritten.")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err = os.Rename(source, target); err != nil {
		return "", err
	}
	return target, nil
}

func migrateStoredSession() (string, error) {
	value, err := keyring.Get(previousStateName, "codeforces.com")
	if err != nil {
		return "", err
	}
	// Validate before creating a new keychain entry. Never fall back to plaintext.
	if _, err = DecodeSession([]byte(value)); err != nil {
		return "", err
	}
	if err = keyring.Set(secretService, "codeforces.com", value); err != nil {
		return "", err
	}
	// Leave the original available to older installs until explicit sign-out.
	return value, nil
}
func forgetPreviousSession() error {
	err := keyring.Delete(previousStateName, "codeforces.com")
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
