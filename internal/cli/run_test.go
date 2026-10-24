package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/acstride-labs/codeforces-client/internal/core"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	}{{[]string{"capabilities", "--request-id", "req-1"}, 0, ""}, {[]string{"version"}, 0, ""}, {[]string{"--version"}, 0, ""}, {[]string{"help"}, 0, ""}, {[]string{"doctor"}, 0, ""}, {[]string{"submit"}, 2, "experimental_disabled"}, {[]string{"session", "check"}, 2, "experimental_disabled"}, {[]string{"session", "check", "--experimental"}, 3, "session_unavailable"}, {[]string{"contest", "info", "../1"}, 2, "invalid_argument"}, {[]string{"standings", "list", "1", "--count", "0"}, 2, "invalid_argument"}, {[]string{"receipt", "abandon", strings.Repeat("a", 32)}, 2, "invalid_argument"}, {[]string{"--password", "SECRET"}, 2, "invalid_argument"}, {[]string{"capabilities", "junk"}, 2, "invalid_argument"}, {[]string{"api", "enter"}, 2, "invalid_argument"}} {
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

func TestStructuredEditorProblemProtocol(t *testing.T) {
	client := core.New()
	client.ReadPublic = func(context.Context, string) (core.Page, error) {
		return core.Page{URL: core.Host + "/contest/1/problem/A", Status: 200, HTML: `<div class="problem-statement"><div class="title">A. Sum</div><div class="sample-test"><div class="input"><pre>2 3</pre></div><div class="output"><pre>5</pre></div></div></div>`}, nil
	}
	secret := &secrets{}
	app := &App{Client: client, Secrets: secret}
	var out bytes.Buffer
	assets := t.TempDir()
	exit := app.Run(context.Background(), []string{"problem", "read", "1", "A", "--assets-dir", assets, "--request-id", "read-1"}, strings.NewReader(""), &out, &out)
	if exit != 0 || secret.calls != 0 {
		t.Fatalf("exit=%d secrets=%d %s", exit, secret.calls, out.String())
	}
	var result struct {
		Protocol  int                `json:"protocol"`
		RequestID string             `json:"requestId"`
		OK        bool               `json:"ok"`
		Data      core.EditorProblem `json:"data"`
	}
	if e := json.Unmarshal(out.Bytes(), &result); e != nil || !result.OK || result.RequestID != "read-1" || result.Data.ContestID != 1 || result.Data.Samples[0].Output != "5\n" {
		t.Fatal(out.String(), e)
	}
}

func TestConfirmedAccountChangeStopsBeforeNetwork(t *testing.T) {
	file := filepath.Join(t.TempDir(), "main.py")
	source := []byte("print(1)\n")
	if e := os.WriteFile(file, source, 0600); e != nil {
		t.Fatal(e)
	}
	session := core.Session{Protocol: 1, Handle: "changed", ExpiresAt: time.Now().Add(time.Hour).Unix(), Cookies: []*http.Cookie{{Name: "JSESSIONID", Value: "fixture", Domain: "codeforces.com", Path: "/", Secure: true}}}
	raw, _ := json.Marshal(session)
	app := &App{Client: core.New(), Secrets: &secrets{}}
	var out bytes.Buffer
	exit := app.Run(context.Background(), []string{"submit", "--experimental", "--confirm", "--session-stdin", "--contest", "1", "--problem", "A", "--file", file, "--language-id", "31", "--expected-sha256", core.SourceHash(source), "--expected-handle", "confirmed"}, bytes.NewReader(raw), &out, &out)
	var result Envelope
	if json.Unmarshal(out.Bytes(), &result) != nil || exit != 3 || result.Error == nil || result.Error.Code != "account_mismatch" {
		t.Fatal(exit, out.String())
	}
}
