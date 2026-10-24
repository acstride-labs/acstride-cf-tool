package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ExternalPublicHelper adapts the existing bundled anonymous curl_cffi reader.
// Only an explicitly supplied absolute executable is run, with no shell and no cookies.
func ExternalPublicHelper(executable string) func(context.Context, string) (Page, error) {
	return func(ctx context.Context, path string) (Page, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if !filepath.IsAbs(executable) {
			return Page{}, Fail("invalid_argument", "The public helper must be an absolute executable path.")
		}
		if !regexp.MustCompile(`^/contest/[1-9][0-9]{0,8}/problem/[A-Z][0-9]{0,2}$`).MatchString(path) {
			return Page{}, Fail("invalid_argument", "The helper only accepts public problem paths.")
		}
		input, _ := json.Marshal(map[string]string{"path": path})
		command := exec.CommandContext(ctx, executable)
		command.Stdin = bytes.NewReader(input)
		command.WaitDelay = time.Second
		for _, v := range os.Environ() {
			key := strings.SplitN(v, "=", 2)[0]
			if !regexp.MustCompile(`(?i)^(PYTHON|PYINSTALLER_|_PYI_|IMPERSONATE_|CONDA|VIRTUAL_ENV|LD_|DYLD_|SSL_CERT_FILE|SSL_CERT_DIR|REQUESTS_CA_BUNDLE|CURL_CA_BUNDLE)`).MatchString(key) {
				command.Env = append(command.Env, v)
			}
		}
		command.Env = append(command.Env, "PYINSTALLER_RESET_ENVIRONMENT=1")
		out := &bytes.Buffer{}
		command.Stdout = out
		command.Stderr = io.Discard
		if e := command.Run(); e != nil {
			if ctx.Err() != nil {
				return Page{}, ctx.Err()
			}
			return Page{}, Fail("helper_failed", "The anonymous public helper failed. Verify the bundled executable for this platform.")
		}
		return parsePublicHelper(out.Bytes(), path)
	}
}

// BundledPublicHelper resolves only immutable install-relative transport assets.
func BundledPublicHelper() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "codeforces-workbench-downloader"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helper := filepath.Join(filepath.Dir(exe), "..", "downloader", "codeforces-workbench-downloader", name)
	info, err := os.Stat(helper)
	if err != nil || !info.Mode().IsRegular() {
		return "", Fail("helper_failed", "The bundled public transport is missing. Reinstall the matching editor package.")
	}
	return filepath.Abs(helper)
}

func parsePublicHelper(raw []byte, path string) (Page, error) {
	var data struct {
		HTML   string `json:"html"`
		URL    string `json:"url"`
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Error != "" {
		return Page{}, Fail("helper_failed", "The anonymous public helper returned an invalid response.")
	}
	if data.URL != Host+path {
		return Page{}, Fail("unexpected_redirect", "The anonymous helper returned a different resource.")
	}
	if verification(data.HTML) {
		return Page{}, Fail("verification_required", "Codeforces requires site verification. Use the official website.")
	}
	if data.Status == 403 {
		return Page{}, Fail("access_denied", "Codeforces refused the anonymous request.")
	}
	if data.Status == 404 {
		return Page{}, Fail("not_found", "Problem is unavailable or unreleased.")
	}
	if data.Status == 429 {
		return Page{}, &Error{Code: "rate_limited", Message: "Codeforces rate limit reached.", Retryable: true}
	}
	if data.Status != 200 {
		return Page{}, Fail("http_error", "Anonymous download returned an HTTP error.")
	}
	return Page{HTML: data.HTML, URL: data.URL, Status: data.Status}, nil
}
