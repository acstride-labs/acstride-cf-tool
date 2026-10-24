package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const statement = `<div class="problem-statement"><div class="header"><div class="title">A. Sum</div><div class="time-limit">time limit 1 second</div><div class="memory-limit">256 MB</div></div><p onclick="alert(1)">Add $$$a+b$$$.</p><script>evil()</script><img src="javascript:evil()" onerror="x"><a href="https://evil.test">link</a><div class="sample-test"><div class="input"><pre><div class="test-example-line">2 3</div><div>4 5</div></pre></div><div class="output"><pre>5<br>9</pre></div></div></div>`
const form = `<div class="lang-chooser"><a href="/profile/tester">tester</a><a href="/logout?csrf=x">Logout</a></div><script>var handle = "tester";</script><input name="csrf_token" value="token12345"><select name="programTypeId"><option value="54">GNU G++17</option><option value="31">Python 3</option></select>`

func serverClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c := New()
	c.base = s.URL
	c.interval = 0
	return c
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil || Classify(e).Code != want {
		t.Fatalf("error %v, want %s", e, want)
	}
}
func TestParseSanitizesAndPreservesSamples(t *testing.T) {
	p, e := ParseProblem(Page{HTML: statement}, 1, "A")
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Samples) != 1 || p.Samples[0].Input != "2 3\n4 5\n" || p.Samples[0].Output != "5\n9\n" {
		t.Fatalf("%+v", p.Samples)
	}
	for _, bad := range []string{"onclick", "onerror", "javascript:", "<script", "evil.test"} {
		if strings.Contains(p.StatementHTML, bad) {
			t.Fatal(bad)
		}
	}
}
func TestParseOldPREAndEntities(t *testing.T) {
	p, e := ParseProblem(Page{HTML: `<div class="problem-statement"><div class="title">A</div><div class="sample-test"><div class="input"><pre>&lt;&amp;<br/> x</pre></div><div class="output"><pre> y</pre></div></div></div>`}, 1, "A")
	if e != nil {
		t.Fatal(e)
	}
	if p.Samples[0].Input != "<&\n x\n" {
		t.Fatal(p.Samples)
	}
}
func TestParseMissingAndMismatched(t *testing.T) {
	if verification(`<div class="problem-statement"><p>Wait just a moment.</p></div>`) {
		t.Fatal("ordinary statement text was treated as a challenge")
	}
	_, e := ParseProblem(Page{HTML: "login"}, 1, "A")
	code(t, e, "problem_not_ready")
	_, e = ParseProblem(Page{HTML: `<div class="problem-statement"><div class="title">A</div><div class="sample-test"><div class="input"><pre>1</pre></div></div></div>`}, 1, "A")
	code(t, e, "parse_error")
}
func TestCSRFAndHandle(t *testing.T) {
	if v, e := CSRF(form); e != nil || v != "token12345" {
		t.Fatal(v, e)
	}
	if v, e := Handle(form); e != nil || v != "tester" {
		t.Fatal(v, e)
	}
	_, e := Handle(`<a href="/profile/tester">tester</a>`)
	code(t, e, "not_authenticated")
	v, e := ParseLanguages(form)
	if e != nil || len(v) != 2 {
		t.Fatal(v, e)
	}
	v, e = ParseLanguages(`<select name="programTypeId"><option value="54" disabled>C++</option><option value="31">Python</option></select>`)
	if e != nil || len(v) != 1 || v[0].ID != "31" {
		t.Fatal(v, e)
	}
}
func TestTransportClassification(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{{403, "<title>Just a moment</title>", "verification_required"}, {403, "denied", "access_denied"}, {429, "slow", "rate_limited"}, {404, "none", "not_found"}, {500, "bad", "http_error"}} {
		t.Run(tc.want, func(t *testing.T) {
			c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) })
			_, e := c.Request(context.Background(), "GET", "/", nil)
			code(t, e, tc.want)
		})
	}
}
func TestRedirectNoCredentialReplay(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer other.Close()
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 307) })
	_, e := c.Request(context.Background(), "POST", "/enter", url.Values{"password": {"secret"}})
	if e == nil || received.Load() != 0 {
		t.Fatal("credential redirect was followed")
	}
}
func TestCancellationAndTimeout(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := c.Request(ctx, "GET", "/", nil)
	code(t, e, "cancelled")
	c = serverClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c.HTTP.Timeout = 20 * time.Millisecond
	_, e = c.Request(context.Background(), "GET", "/", nil)
	code(t, e, "timeout")
}
func TestAPIRejectsHTMLAndFailure(t *testing.T) {
	for _, body := range []string{"<html>oops</html>", `{"status":"FAILED","comment":"SECRET"}`} {
		c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })
		var v any
		e := c.API(context.Background(), "contest.list", nil, &v)
		if e == nil || strings.Contains(e.Error(), "SECRET") {
			t.Fatal(e)
		}
	}
}
func TestAnonymousCacheAndNoSolutionOverwrite(t *testing.T) {
	var requests atomic.Int32
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if len(r.Cookies()) > 0 {
			t.Error("anonymous sent cookies")
		}
		io.WriteString(w, statement)
	})
	dir := t.TempDir()
	solution := filepath.Join(dir, "main.cpp")
	os.WriteFile(solution, []byte("my work"), 0600)
	for range 2 {
		if _, e := c.Fetch(context.Background(), 1, "A", dir, false); e != nil {
			t.Fatal(e)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("cache not used")
	}
	b, _ := os.ReadFile(solution)
	if string(b) != "my work" {
		t.Fatal("solution overwritten")
	}
	if _, e := c.Fetch(context.Background(), 1, "A", dir, true); e != nil {
		t.Fatal(e)
	}
	if requests.Load() != 2 {
		t.Fatal("refresh did not request")
	}
}
func TestOutputSymlinkCannotEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(dir, "contests")); e != nil {
		t.Skip(e)
	}
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, statement) })
	_, e := c.Fetch(context.Background(), 1, "A", dir, false)
	if e == nil {
		t.Fatal("escaped output root")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) > 0 {
		t.Fatal("wrote outside output root")
	}
}
func TestDownloadPartialFailure(t *testing.T) {
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			io.WriteString(w, `{"status":"OK","result":{"contest":{"id":1,"phase":"FINISHED"},"problems":[{"index":"A"},{"index":"B"}],"rows":[]}}`)
		case strings.HasSuffix(r.URL.Path, "/A"):
			io.WriteString(w, statement)
		default:
			w.WriteHeader(404)
		}
	})
	n := 0
	items, e := c.Download(context.Background(), 1, t.TempDir(), false, func(v ImportItem) { n++ })
	code(t, e, "partial_failure")
	if len(items) != 2 || items[0].Problem == nil || items[1].Error == nil || n != 2 {
		t.Fatalf("%+v", items)
	}
}
func TestValidation(t *testing.T) {
	for _, s := range []string{"../A", "A/../../", "", "G1234"} {
		_, e := ProblemIndex(s)
		code(t, e, "invalid_argument")
	}
	for _, s := range []string{"0", "-1", "https://evil.test/contest/1", "1?x"} {
		_, e := ContestID(s)
		code(t, e, "invalid_argument")
	}
	if n, e := ContestID("https://codeforces.com/contest/123/"); e != nil || n != 123 {
		t.Fatal(n, e)
	}
}
func TestSessionValidation(t *testing.T) {
	s := Session{Protocol: 1, Handle: "tester", ExpiresAt: time.Now().Add(time.Hour).Unix(), Cookies: []*http.Cookie{{Name: "JSESSIONID", Value: "secret"}}}
	b, _ := json.Marshal(s)
	if _, e := DecodeSession(b); e != nil {
		t.Fatal(e)
	}
	s.Cookies[0].Domain = "evil.test"
	b, _ = json.Marshal(s)
	_, e := DecodeSession(b)
	code(t, e, "invalid_session")
	s.Cookies[0].Domain = ""
	s.ExpiresAt = 1
	b, _ = json.Marshal(s)
	_, e = DecodeSession(b)
	code(t, e, "invalid_session")
}
func TestUnknownReceiptBlocksDuplicateAndReconcilesExactSource(t *testing.T) {
	source := []byte("print(1)\n")
	var posts atomic.Int32
	var sent atomic.Bool
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			posts.Add(1)
			sent.Store(true)
			w.WriteHeader(503)
		case strings.HasPrefix(r.URL.Path, "/api/"):
			if !sent.Load() {
				io.WriteString(w, `{"status":"OK","result":[]}`)
			} else {
				io.WriteString(w, `{"status":"OK","result":[{"id":123,"contestId":1,"creationTimeSeconds":`+strconv64(time.Now().Unix())+`,"problem":{"index":"A"},"programmingLanguage":"Python 3","author":{"members":[{"handle":"tester"}]}}]}`)
			}
		case strings.Contains(r.URL.Path, "/submission/"):
			io.WriteString(w, `<pre id="program-source-text">print(1)`+"\n"+`</pre>`)
		default:
			io.WriteString(w, form)
		}
	})
	r := Receipts{Dir: filepath.Join(t.TempDir(), "state")}
	s := Session{Protocol: 1, Handle: "tester"}
	v, e := c.Send(context.Background(), r, s, 1, "A", "31", source)
	code(t, e, "submission_unknown")
	if v.ID == "" {
		t.Fatal("no durable receipt")
	}
	stored, e := r.Load(v.ID)
	if e != nil || stored.State != "unknown" {
		t.Fatal(stored, e)
	}
	_, e = c.Send(context.Background(), r, s, 1, "A", "31", source)
	code(t, e, "submission_pending")
	if posts.Load() != 1 {
		t.Fatal("duplicate post")
	}
	v, e = c.Reconcile(context.Background(), r, v.ID, 0)
	if e != nil || v.State != "confirmed" || v.SubmissionID != 123 {
		t.Fatal(v, e)
	}
}
func strconv64(n int64) string { return fmt.Sprint(n) }
func TestReceiptNoNewestRecordAssumption(t *testing.T) {
	r := Receipts{Dir: filepath.Join(t.TempDir(), "state")}
	v := Receipt{Protocol: 1, ID: strings.Repeat("a", 32), State: "unknown", Handle: "tester", ContestID: 1, Index: "A", Language: "Python 3", SHA256: SourceHash([]byte("original")), CreatedAt: time.Now()}
	r.save(v)
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			io.WriteString(w, `{"status":"OK","result":[{"id":999,"contestId":1,"creationTimeSeconds":`+fmt.Sprint(time.Now().Unix())+`,"problem":{"index":"A"},"programmingLanguage":"Python 3","author":{"members":[{"handle":"tester"}]}}]}`)
		} else if strings.Contains(r.URL.Path, "/submission/") {
			io.WriteString(w, `<pre id="program-source-text">different</pre>`)
		} else {
			io.WriteString(w, form)
		}
	})
	_, e := c.Reconcile(context.Background(), r, v.ID, 0)
	code(t, e, "submission_unknown")
}
func TestAbandonRequiresUnknownAndKeepsSnapshot(t *testing.T) {
	r := Receipts{Dir: filepath.Join(t.TempDir(), "state")}
	v := Receipt{Protocol: 1, ID: strings.Repeat("b", 32), State: "unknown", Handle: "tester", ContestID: 1, Index: "A"}
	r.save(v)
	got, e := r.Abandon(context.Background(), v.ID)
	if e != nil || got.State != "abandoned" {
		t.Fatal(got, e)
	}
	_, e = r.Abandon(context.Background(), v.ID)
	code(t, e, "invalid_receipt")
}
func TestLoginNeverConfirmsChallenge(t *testing.T) {
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<title>Just a moment</title>") })
	_, e := c.Login(context.Background(), "tester", "password")
	code(t, e, "verification_required")
}
func TestLanguageAndAccountMismatchNeverPosts(t *testing.T) {
	for _, tc := range []struct{ handle, language, want string }{{"other", "31", "account_mismatch"}, {"tester", "999", "invalid_language"}} {
		var posts int
		c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				posts++
			}
			io.WriteString(w, form)
		})
		_, e := c.Send(context.Background(), Receipts{Dir: filepath.Join(t.TempDir(), "state")}, Session{Handle: tc.handle}, 1, "A", tc.language, []byte("source"))
		code(t, e, tc.want)
		if posts != 0 {
			t.Fatal("unexpected post")
		}
	}
}
