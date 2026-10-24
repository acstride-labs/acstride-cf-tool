package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/acstride-labs/acstride-cf-tool/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type secrets struct{ calls int }

func (s *secrets) Get() (core.Session, error) {
	s.calls++
	return core.Session{}, core.Fail("session_unavailable", "No session.")
}
func (s *secrets) Set(core.Session) error { s.calls++; return nil }
func (s *secrets) Delete() error          { s.calls++; return nil }
func TestProtocolAndGates(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		want      int
		errorCode string
	}{{[]string{"capabilities", "--request-id", "req-1"}, 0, ""}, {[]string{"version"}, 0, ""}, {[]string{"--version"}, 0, ""}, {[]string{"help"}, 0, ""}, {[]string{"doctor"}, 0, ""}, {[]string{"submit"}, 2, "experimental_disabled"}, {[]string{"session", "check"}, 2, "experimental_disabled"}, {[]string{"session", "check", "--experimental"}, 3, "session_unavailable"}, {[]string{"contest", "info", "../1"}, 2, "invalid_argument"}, {[]string{"standings", "list", "1", "--count", "1000"}, 2, "invalid_argument"}, {[]string{"receipt", "abandon", strings.Repeat("a", 32)}, 2, "invalid_argument"}, {[]string{"--password", "SECRET"}, 2, "invalid_argument"}, {[]string{"capabilities", "junk"}, 2, "invalid_argument"}, {[]string{"api", "enter"}, 2, "invalid_argument"}} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out, log bytes.Buffer
			s := &secrets{}
			app := &App{Client: core.New(), Secrets: s}
			n := app.Run(context.Background(), tc.args, strings.NewReader(""), &out, &log)
			if n != tc.want {
				t.Fatalf("exit %d: %s", n, out.String())
			}
			var v Envelope
			if json.Unmarshal(out.Bytes(), &v) != nil || v.Protocol != 1 || v.Type != "result" {
				t.Fatal(out.String())
			}
			if tc.errorCode != "" && (v.Error == nil || v.Error.Code != tc.errorCode) {
				t.Fatal(out.String())
			}
			if strings.Contains(out.String()+log.String(), "SECRET") {
				t.Fatal("secret leaked")
			}
			if tc.args[0] == "capabilities" && s.calls != 0 {
				t.Fatal("anonymous command used keyring")
			}
		})
	}
}
func TestSourceChangedRejectedBeforeSessionAccess(t *testing.T) {
	s := &secrets{}
	app := &App{Client: core.New(), Secrets: s}
	var out bytes.Buffer
	file := filepath.Join(t.TempDir(), "main.py")
	if e := os.WriteFile(file, []byte("new code"), 0600); e != nil {
		t.Fatal(e)
	}
	n := app.Run(context.Background(), []string{"submit", "--experimental", "--confirm", "--contest", "1", "--problem", "A", "--file", file, "--language-id", "31", "--expected-sha256", core.SourceHash([]byte("old code"))}, strings.NewReader(""), &out, &out)
	if n == 0 || s.calls != 0 {
		t.Fatal("unexpected session access")
	}
	var env Envelope
	json.Unmarshal(out.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "source_changed" {
		t.Fatal(out.String())
	}
}
