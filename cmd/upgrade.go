package cmd

import "errors"

// Bundled binaries are updated by their host extension, never replaced in place.
func Upgrade() error {
	return errors.New("Self-update is disabled. Install a verified ACStride release or update the VS Code extension")
}
