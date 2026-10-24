// Package cli implements protocol v1. It never prints credentials, HTML errors or ANSI escapes.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/acstride-labs/acstride-cf-tool/internal/core"
)

var Version = "0.5.0"

type Envelope struct {
	Protocol  int         `json:"protocol"`
	RequestID string      `json:"requestId"`
	Type      string      `json:"type"`
	OK        bool        `json:"ok"`
	Data      any         `json:"data,omitempty"`
	Error     *core.Error `json:"error,omitempty"`
}
type App struct {
	Client  *core.Client
	Secrets core.SecretStore
}
type options struct {
	expectedHandle                                                                        string
	requestID, output, stateDir, handle, contest, problem, file, languageID, expectedHash string
	publicHelper                                                                          string
	assetsDir                                                                             string
	bundled                                                                               bool
	allRows                                                                               bool
	refresh, stream, experimental, confirm, sessionStdin, credentialsStdin, store         bool
	count, from                                                                           int
	submissionID                                                                          int64
	timeout                                                                               time.Duration
}

func parse(args []string) (options, []string, error) {
	var o options
	f := flag.NewFlagSet("acstride-cf", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.requestID, "request-id", "", "request correlation ID")
	f.StringVar(&o.output, "output", "", "cache output root")
	f.StringVar(&o.stateDir, "state-dir", "", "private receipt directory")
	f.StringVar(&o.handle, "handle", "", "Codeforces handle")
	f.StringVar(&o.expectedHandle, "expected-handle", "", "account confirmed by the caller before submission")
	f.StringVar(&o.contest, "contest", "", "contest ID")
	f.StringVar(&o.problem, "problem", "", "problem index")
	f.StringVar(&o.file, "file", "", "exact source path")
	f.StringVar(&o.languageID, "language-id", "", "official language ID")
	f.StringVar(&o.expectedHash, "expected-sha256", "", "confirmed source SHA256")
	f.StringVar(&o.publicHelper, "public-helper", "", "absolute bundled anonymous downloader path")
	f.StringVar(&o.assetsDir, "assets-dir", "", "explicit statement image cache root")
	f.BoolVar(&o.bundled, "bundled", false, "require the install-relative anonymous transport")
	f.BoolVar(&o.allRows, "all-rows", false, "read all ranking rows")
	f.BoolVar(&o.refresh, "refresh", false, "refresh cache")
	f.BoolVar(&o.stream, "stream", false, "NDJSON progress")
	f.BoolVar(&o.experimental, "experimental", false, "enable website operations")
	f.BoolVar(&o.confirm, "confirm", false, "explicit send or abandon confirmation")
	f.BoolVar(&o.sessionStdin, "session-stdin", false, "read a transient session on stdin")
	f.BoolVar(&o.credentialsStdin, "credentials-stdin", false, "read credentials on stdin")
	f.BoolVar(&o.store, "store", false, "store session in OS keyring")
	f.Bool("json", true, "protocol v1 JSON output")
	f.Bool("non-interactive", true, "never prompt")
	f.IntVar(&o.count, "count", 100, "positive result count")
	f.IntVar(&o.from, "from", 1, "first ranking row")
	f.Int64Var(&o.submissionID, "submission-id", 0, "exact official submission ID")
	f.DurationVar(&o.timeout, "timeout", 120*time.Second, "total operation deadline, 1s..10m")
	// Allow flags before or after positional command arguments, without a shell.
	positional := []string{}
	flags := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			positional = append(positional, a)
			continue
		}
		name := strings.TrimPrefix(strings.SplitN(a, "=", 2)[0], "--")
		fl := f.Lookup(name)
		if fl == nil {
			return o, nil, core.Fail("invalid_argument", "Unknown option: --"+name)
		}
		flags = append(flags, a)
		if !strings.Contains(a, "=") {
			if b, ok := fl.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				i++
				if i >= len(args) {
					return o, nil, core.Fail("invalid_argument", "An option value is missing.")
				}
				flags = append(flags, args[i])
			}
		}
	}
	if e := f.Parse(flags); e != nil {
		return o, nil, core.Fail("invalid_argument", "Invalid option value.")
	}
	if len(o.requestID) > 64 || strings.ContainsAny(o.requestID, "\r\n") {
		return o, nil, core.Fail("invalid_argument", "Invalid request ID.")
	}
	if o.requestID == "" {
		o.requestID = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	if o.count < 1 || o.from < 1 || o.timeout < time.Second || o.timeout > 10*time.Minute {
		return o, nil, core.Fail("invalid_argument", "Count, offset or timeout is out of range.")
	}
	if o.sessionStdin && o.credentialsStdin {
		return o, nil, core.Fail("invalid_argument", "Choose session stdin or credential stdin, not both.")
	}
	return o, positional, nil
}
func Run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer) int {
	client := core.New()
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	client.Pace = core.SharedPacer(filepath.Join(cache, "acstride-cf-tool", "network"), 2100*time.Millisecond)
	return (&App{Client: client, Secrets: core.KeyringStore{}}).Run(ctx, args, in, out, diagnostics)
}
func (a *App) Run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer) int {
	if len(args) == 1 {
		if args[0] == "--version" {
			args = []string{"version"}
		}
		if args[0] == "--help" {
			args = []string{"help"}
		}
	}
	o, pos, e := parse(args)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(true)
	send := func(kind string, data any, err error) error {
		v := Envelope{Protocol: 1, RequestID: o.requestID, Type: kind, OK: err == nil, Data: data}
		if err != nil {
			v.Error = core.Classify(err)
		}
		return enc.Encode(v)
	}
	if e != nil {
		_ = send("result", nil, e)
		return 2
	}
	if len(pos) == 1 && pos[0] == "serve" {
		helper := o.publicHelper
		if o.bundled {
			helper, e = core.BundledPublicHelper()
			if e != nil {
				_ = send("result", nil, e)
				return 1
			}
		}
		return a.serve(ctx, in, out, helper)
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	if o.bundled {
		o.publicHelper, e = core.BundledPublicHelper()
		if e != nil {
			_ = send("result", nil, e)
			return 1
		}
	}
	if o.publicHelper != "" {
		a.Client.ReadPublic = core.ExternalPublicHelper(o.publicHelper)
	}
	var progress func(core.ImportItem)
	if o.stream {
		progress = func(v core.ImportItem) {
			if err := send("progress", v, nil); err != nil {
				cancel()
			}
		}
	}
	result, e := a.execute(ctx, o, pos, in, progress)
	if err := send("result", result, e); err != nil {
		fmt.Fprintln(diagnostics, "Could not write protocol response.")
		return 1
	}
	if e == nil {
		return 0
	}
	code := core.Classify(e).Code
	switch code {
	case "invalid_argument", "invalid_language", "experimental_disabled":
		return 2
	case "not_authenticated", "session_unavailable", "account_mismatch", "login_failed":
		return 3
	case "verification_required", "access_denied":
		return 4
	case "submission_unknown", "submission_pending":
		return 5
	case "cancelled":
		return 130
	default:
		return 1
	}
}
func input(in io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(in, 64001))
	if e != nil || len(b) > 64000 {
		return nil, core.Fail("invalid_argument", "Stdin exceeds the input limit.")
	}
	return b, nil
}
func (a *App) session(o options, in io.Reader) (core.Session, error) {
	var s core.Session
	var e error
	if o.sessionStdin {
		var b []byte
		b, e = input(in)
		if e == nil {
			s, e = core.DecodeSession(b)
		}
	} else {
		s, e = a.Secrets.Get()
	}
	if e != nil {
		return s, e
	}
	return s, a.Client.UseSession(s)
}
func (a *App) execute(ctx context.Context, o options, p []string, in io.Reader, progress func(core.ImportItem)) (any, error) {
	if len(p) == 0 || len(p) == 1 && (p[0] == "help" || p[0] == "-h") {
		return map[string]any{"name": "acstride-cf", "version": Version, "commands": []string{"serve --bundled", "contest problems ID", "capabilities", "doctor", "contest info ID", "contest download ID --output DIR", "problem fetch ID INDEX", "problem read ID INDEX --assets-dir DIR", "problem page ID INDEX", "submissions list --handle HANDLE [--contest ID]", "standings list ID", "languages list --contest ID --experimental", "session login --credentials-stdin --experimental --store", "session check --experimental", "session forget", "receipt show ID", "receipt reconcile ID --experimental", "receipt abandon ID --confirm", "submission watch --contest ID --handle HANDLE --submission-id ID", "submit --contest ID --problem INDEX --file FILE --language-id ID --experimental --confirm"}}, nil
	}
	if len(p) == 1 && (p[0] == "--version" || p[0] == "version") {
		return map[string]string{"version": Version}, nil
	}
	if len(p) == 1 && p[0] == "capabilities" {
		return map[string]any{"version": Version, "protocol": 1, "anonymous": []string{"contest.problems", "download.stream", "contest.info", "contest.download", "problem.fetch", "problem.read", "problem.page", "submissions.list", "standings.list", "submission.watch", "api"}, "experimental": []string{"session.login", "session.check", "languages.list", "submit", "receipt.reconcile"}, "automaticSubmissionVerified": false, "nonInteractive": true, "selfUpdate": false, "secretStorage": "OS keyring; transient stdin session also supported", "statementFormat": "sanitized-html-and-text"}, nil
	}
	if len(p) == 1 && p[0] == "doctor" {
		dir, e := core.StateDir()
		if e != nil {
			return nil, e
		}
		return map[string]any{"version": Version, "protocol": 1, "networkTested": false, "stateDirectory": dir, "keyring": "checked only when explicitly used", "legacyPasswordMigration": "disabled; legacy ~/.cf/session is not read by modern commands"}, nil
	}
	command := strings.Join(p[:min(2, len(p))], " ")
	getID := func(s string) (int, error) {
		if s == "" {
			s = o.contest
		}
		return core.ContestID(s)
	}
	if p[0] == "api" && len(p) == 2 {
		switch p[1] {
		case "contest.list":
			var v []core.Contest
			e := a.Client.API(ctx, "contest.list", url.Values{"gym": {"false"}}, &v)
			return v, e
		case "contest.status":
			id, e := getID("")
			if e != nil {
				return nil, e
			}
			return a.Client.Submissions(ctx, id, o.handle, o.count)
		case "contest.standings":
			id, e := getID("")
			if e != nil {
				return nil, e
			}
			if o.allRows {
				var result core.Standings
				e := a.Client.API(ctx, "contest.standings", url.Values{"contestId": {strconv.Itoa(id)}}, &result)
				return result, e
			}
			return a.Client.Rankings(ctx, id, o.handle, o.from, o.count)
		}
		return nil, core.Fail("invalid_argument", "This API method is not exposed by the read-only bridge.")
	}
	switch command {
	case "contest problems":
		if len(p) != 3 {
			return nil, core.Fail("invalid_argument", "Specify the contest ID.")
		}
		id, e := getID(p[2])
		if e != nil {
			return nil, e
		}
		return a.Client.ContestProblems(ctx, id)
	case "contest info":
		if len(p) != 3 {
			return nil, core.Fail("invalid_argument", "Specify the contest ID.")
		}
		id, e := getID(p[2])
		if e != nil {
			return nil, e
		}
		return a.Client.Contest(ctx, id)
	case "contest download":
		if len(p) != 3 {
			return nil, core.Fail("invalid_argument", "Specify the contest ID.")
		}
		id, e := getID(p[2])
		if e != nil {
			return nil, e
		}
		return a.Client.Download(ctx, id, o.output, o.refresh, progress)
	case "problem fetch", "problem page", "problem read":
		if len(p) != 4 {
			return nil, core.Fail("invalid_argument", "Specify the contest ID and problem index.")
		}
		id, e := getID(p[2])
		if e != nil {
			return nil, e
		}
		idx, e := core.ProblemIndex(p[3])
		if e != nil {
			return nil, e
		}
		if p[1] == "read" {
			return a.Client.ReadProblem(ctx, id, idx, o.assetsDir)
		}
		if p[1] == "page" {
			return a.Client.ProblemPage(ctx, id, idx)
		}
		return a.Client.Fetch(ctx, id, idx, o.output, o.refresh)
	case "submissions list":
		if len(p) != 2 {
			return nil, core.Fail("invalid_argument", "Use --contest to select a contest.")
		}
		id := 0
		var e error
		if o.contest != "" {
			id, e = getID("")
			if e != nil {
				return nil, e
			}
		}
		return a.Client.Submissions(ctx, id, o.handle, o.count)
	case "standings list":
		if len(p) != 3 {
			return nil, core.Fail("invalid_argument", "Specify the contest ID.")
		}
		id, e := getID(p[2])
		if e != nil {
			return nil, e
		}
		return a.Client.Rankings(ctx, id, o.handle, o.from, o.count)
	case "submission watch":
		id, e := getID("")
		if e != nil {
			return nil, e
		}
		if o.submissionID <= 0 {
			return nil, core.Fail("invalid_argument", "Specify an exact submission ID.")
		}
		for {
			items, e := a.Client.Submissions(ctx, id, o.handle, 100)
			if e != nil {
				return nil, e
			}
			found := false
			for _, v := range items {
				if v.ID == o.submissionID {
					found = true
					if v.Verdict != "" && v.Verdict != "TESTING" {
						return v, nil
					}
				}
			}
			if !found {
				return nil, core.Fail("not_found", "The submission is not in the recent public records.")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
	case "session forget":
		if len(p) != 2 {
			return nil, core.Fail("invalid_argument", "Unexpected argument.")
		}
		return map[string]bool{"removed": true}, a.Secrets.Delete()
	case "session login", "session check", "languages list":
		if !o.experimental {
			return nil, core.Fail("experimental_disabled", "Website operations require --experimental and may be blocked by site verification.")
		}
		if command == "session login" {
			if !o.credentialsStdin || !o.store {
				return nil, core.Fail("invalid_argument", "Login requires --credentials-stdin --store. Passwords are never accepted in arguments or saved.")
			}
			b, e := input(in)
			if e != nil {
				return nil, e
			}
			var v struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if json.Unmarshal(b, &v) != nil {
				return nil, core.Fail("invalid_argument", "Expected credentials JSON on stdin.")
			}
			s, e := a.Client.Login(ctx, v.Username, v.Password)
			v.Password = ""
			clear(b)
			if e != nil {
				return nil, e
			}
			if e = a.Secrets.Set(s); e != nil {
				return nil, e
			}
			return map[string]any{"handle": s.Handle, "stored": true}, nil
		}
		if _, e := a.session(o, in); e != nil {
			return nil, e
		}
		if command == "session check" {
			s, e := a.Client.Session(ctx)
			return map[string]any{"handle": s.Handle, "authenticated": e == nil}, e
		}
		id, e := getID("")
		if e != nil {
			return nil, e
		}
		return a.Client.Languages(ctx, id)
	}
	if p[0] == "receipt" {
		if len(p) != 3 {
			return nil, core.Fail("invalid_argument", "Specify a receipt command and receipt ID.")
		}
		dir := o.stateDir
		if dir == "" {
			var e error
			dir, e = core.StateDir()
			if e != nil {
				return nil, e
			}
		}
		r := core.Receipts{Dir: dir}
		switch p[1] {
		case "show":
			return r.Load(p[2])
		case "abandon":
			if !o.confirm {
				return nil, core.Fail("invalid_argument", "Abandon requires --confirm after checking the official website. It does not prove the submission failed.")
			}
			return r.Abandon(ctx, p[2])
		case "reconcile":
			if !o.experimental {
				return nil, core.Fail("experimental_disabled", "Reconciliation requires --experimental.")
			}
			if _, e := a.session(o, in); e != nil {
				return nil, e
			}
			return a.Client.Reconcile(ctx, r, p[2], o.submissionID)
		}
	}
	if p[0] == "submit" {
		if !o.experimental || !o.confirm {
			return nil, core.Fail("experimental_disabled", "Sending code requires --experimental --confirm. Automatic submission has not been verified on a real account.")
		}
		if len(p) != 1 || o.file == "" || o.languageID == "" || len(o.expectedHash) != 64 {
			return nil, core.Fail("invalid_argument", "Use --contest, --problem, --file, --language-id and --expected-sha256 to bind the exact source snapshot.")
		}
		id, e := getID("")
		if e != nil {
			return nil, e
		}
		idx, e := core.ProblemIndex(o.problem)
		if e != nil {
			return nil, e
		}
		info, e := os.Stat(o.file)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1_000_000 {
			return nil, core.Fail("invalid_argument", "Source must be a regular file containing 1 to 1000000 bytes.")
		}
		file, e := os.Open(o.file)
		if e != nil {
			return nil, e
		}
		source, e := io.ReadAll(io.LimitReader(file, 1_000_001))
		file.Close()
		if e != nil {
			return nil, e
		}
		if core.SourceHash(source) != o.expectedHash {
			return nil, core.Fail("source_changed", "The source no longer matches the confirmed snapshot. Save and review it before sending.")
		}
		s, e := a.session(o, in)
		if e != nil {
			return nil, e
		}
		if o.expectedHandle != "" && !strings.EqualFold(s.Handle, o.expectedHandle) {
			return nil, core.Fail("account_mismatch", "The stored account changed after confirmation. Review the account before sending.")
		}
		dir := o.stateDir
		if dir == "" {
			dir, e = core.StateDir()
			if e != nil {
				return nil, e
			}
		}
		return a.Client.Send(ctx, core.Receipts{Dir: dir}, s, id, idx, o.languageID, source)
	}
	return nil, core.Fail("invalid_argument", "Unknown command. Run help to list supported commands.")
}
