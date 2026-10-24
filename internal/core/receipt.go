package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/net/html"
)

func itoa(n int) string { return strconv.Itoa(n) }

type Receipt struct {
	Protocol     int       `json:"protocol"`
	ID           string    `json:"id"`
	State        string    `json:"state"`
	Handle       string    `json:"handle"`
	ContestID    int       `json:"contestId"`
	Index        string    `json:"index"`
	LanguageID   string    `json:"languageId"`
	Language     string    `json:"language"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"createdAt"`
	Baseline     []int64   `json:"baseline"`
	SubmissionID int64     `json:"submissionId,omitempty"`
}
type Receipts struct{ Dir string }

func StateDir() (string, error) {
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return migrateStateDir(d)
}
func (r Receipts) root() (*os.Root, error) {
	if e := os.MkdirAll(r.Dir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(r.Dir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, Fail("unsafe_path", "Receipt state must be a real private directory.")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, Fail("unsafe_path", "Receipt state directory must be private (0700). Choose a dedicated directory.")
	}
	return os.OpenRoot(r.Dir)
}

var receiptID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (r Receipts) Load(id string) (Receipt, error) {
	if !receiptID.MatchString(id) {
		return Receipt{}, Fail("invalid_argument", "Invalid receipt ID.")
	}
	root, e := r.root()
	if e != nil {
		return Receipt{}, e
	}
	defer root.Close()
	f, e := root.Open(id + ".json")
	if e != nil {
		return Receipt{}, Fail("not_found", "Receipt not found.")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 100000))
	if e != nil {
		return Receipt{}, e
	}
	var v Receipt
	if json.Unmarshal(b, &v) != nil || v.Protocol != 1 || v.ID != id || ValidHandle(v.Handle) != nil || v.ContestID < 1 || v.ContestID > 999999999 || !indexRE.MatchString(v.Index) || v.State != "unknown" && v.State != "confirmed" && v.State != "abandoned" {
		return v, Fail("invalid_receipt", "Invalid receipt.")
	}
	return v, nil
}
func (r Receipts) save(v Receipt) error {
	root, e := r.root()
	if e != nil {
		return e
	}
	defer root.Close()
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return Atomic(root, v.ID+".json", b, 0600)
}
func (r Receipts) lock(ctx context.Context, handle string, id int, index string) (func(), error) {
	root, e := r.root()
	if e != nil {
		return nil, e
	}
	defer root.Close()
	key := "target-" + SourceHash([]byte(strings.ToLower(handle)+"\n"+itoa(id)+"\n"+index))
	f := flock.New(filepath.Join(r.Dir, key+".lock"))
	ok, e := f.TryLockContext(ctx, 100*time.Millisecond)
	if e != nil || !ok {
		return nil, Fail("busy", "Another operation is using this submission target.")
	}
	return func() { _ = f.Unlock() }, nil
}
func (r Receipts) pending(handle string, id int, index string) error {
	entries, e := os.ReadDir(r.Dir)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			v, e := r.Load(strings.TrimSuffix(entry.Name(), ".json"))
			if e != nil {
				return Fail("invalid_receipt", "A receipt is unreadable; inspect local state before submitting again.")
			}
			if strings.EqualFold(v.Handle, handle) && v.ContestID == id && v.Index == index && v.State == "unknown" {
				return Fail("submission_pending", "An uncertain submission exists. Reconcile its receipt before sending again.")
			}
		}
	}
	return nil
}

// Send persists an unknown receipt and source snapshot BEFORE the one and only POST.
// A lost response never triggers a second POST, including after restarting the process.
func (c *Client) Send(ctx context.Context, r Receipts, s Session, id int, index, languageID string, source []byte) (Receipt, error) {
	index, e := ProblemIndex(index)
	if e != nil {
		return Receipt{}, e
	}
	if len(source) == 0 || len(source) > 1_000_000 {
		return Receipt{}, Fail("invalid_argument", "Source must contain 1 to 1000000 bytes.")
	}
	unlock, e := r.lock(ctx, s.Handle, id, index)
	if e != nil {
		return Receipt{}, e
	}
	defer unlock()
	if e = r.pending(s.Handle, id, index); e != nil {
		return Receipt{}, e
	}
	p, e := c.Request(ctx, "GET", "/contest/"+itoa(id)+"/submit", nil)
	if e != nil {
		return Receipt{}, e
	}
	h, e := Handle(p.HTML)
	if e != nil {
		return Receipt{}, e
	}
	if !strings.EqualFold(h, s.Handle) {
		return Receipt{}, Fail("account_mismatch", "Authenticated handle does not match the selected session.")
	}
	csrf, e := CSRF(p.HTML)
	if e != nil {
		return Receipt{}, e
	}
	langs, e := ParseLanguages(p.HTML)
	if e != nil {
		return Receipt{}, e
	}
	language := ""
	for _, v := range langs {
		if v.ID == languageID {
			language = v.Name
		}
	}
	if language == "" {
		return Receipt{}, Fail("invalid_language", "The selected language is not offered by this contest's submission form.")
	}
	baseline, e := c.Submissions(ctx, id, h, 100)
	if e != nil {
		return Receipt{}, e
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return Receipt{}, e
	}
	v := Receipt{Protocol: 1, ID: hex.EncodeToString(nonce), State: "unknown", Handle: h, ContestID: id, Index: index, LanguageID: languageID, Language: language, SHA256: SourceHash(source), CreatedAt: time.Now().UTC(), Baseline: []int64{}}
	for _, sub := range baseline {
		v.Baseline = append(v.Baseline, sub.ID)
	}
	root, e := r.root()
	if e != nil {
		return v, e
	}
	defer root.Close()
	if e = Atomic(root, v.ID+".source", source, 0600); e != nil {
		return v, e
	}
	if e = r.save(v); e != nil {
		return v, e
	}
	_, e = c.Request(ctx, "POST", "/contest/"+itoa(id)+"/submit?csrf_token="+url.QueryEscape(csrf), url.Values{"csrf_token": {csrf}, "action": {"submitSolutionFormSubmitted"}, "submittedProblemIndex": {index}, "programTypeId": {languageID}, "contestId": {itoa(id)}, "source": {string(source)}, "sourceCodeConfirmed": {"true"}, "tabSize": {"4"}, "ftaa": {v.ID[:18]}, "bfaa": {"f1b3f18c715565b589b7823cda7448ce"}, "_tta": {"594"}})
	// Even a 200 response or the text 'submitted successfully' is not a receipt ID.
	if e != nil {
		return v, Fail("submission_unknown", "The submission response was not confirmed. Do not resubmit; reconcile this receipt.")
	}
	return v, Fail("submission_unknown", "The request was sent once. Reconcile the receipt to verify its submission ID and source.")
}
func (c *Client) Reconcile(ctx context.Context, r Receipts, id string, explicitID int64) (Receipt, error) {
	v, e := r.Load(id)
	if e != nil {
		return v, e
	}
	unlock, e := r.lock(ctx, v.Handle, v.ContestID, v.Index)
	if e != nil {
		return v, e
	}
	defer unlock()
	v, e = r.Load(id)
	if e != nil {
		return v, e
	}
	if v.State == "confirmed" {
		return v, nil
	}
	if v.State != "unknown" {
		return v, Fail("invalid_receipt", "This receipt cannot be reconciled.")
	}
	current, e := c.Session(ctx)
	if e != nil {
		return v, e
	}
	if !strings.EqualFold(current.Handle, v.Handle) {
		return v, Fail("account_mismatch", "Use the account which sent this submission.")
	}
	subs, e := c.Submissions(ctx, v.ContestID, v.Handle, 100)
	if e != nil {
		return v, e
	}
	previous := map[int64]bool{}
	for _, n := range v.Baseline {
		previous[n] = true
	}
	matches := []int64{}
	for _, sub := range subs {
		own := false
		for _, m := range sub.Author.Members {
			if strings.EqualFold(m.Handle, v.Handle) {
				own = true
			}
		}
		if !own || previous[sub.ID] || sub.ContestID != v.ContestID || sub.Problem.Index != v.Index || sub.Language != v.Language || sub.Time < v.CreatedAt.Unix()-10 || sub.Time > v.CreatedAt.Add(10*time.Minute).Unix() {
			continue
		}
		if explicitID > 0 && sub.ID != explicitID {
			continue
		}
		page, e := c.Request(ctx, "GET", "/contest/"+itoa(v.ContestID)+"/submission/"+strconv.FormatInt(sub.ID, 10), nil)
		if e != nil {
			return v, e
		}
		doc, e := document(page.HTML)
		if e != nil {
			return v, e
		}
		code := find(doc, func(n *html.Node) bool { return n.Data == "pre" && attr(n, "id") == "program-source-text" })
		// Exact source equality only; timestamps and a newest record are insufficient.
		if code != nil && SourceHash([]byte(text(code))) == v.SHA256 {
			matches = append(matches, sub.ID)
		}
	}
	if len(matches) != 1 {
		return v, Fail("submission_unknown", "No unique exact-source match was found in recent submissions. Check the official website; no retry was sent.")
	}
	v.State = "confirmed"
	v.SubmissionID = matches[0]
	if e = r.save(v); e != nil {
		return v, e
	}
	return v, nil
}

// Abandon is an explicit acknowledgement, not proof that the original POST failed.
func (r Receipts) Abandon(ctx context.Context, id string) (Receipt, error) {
	v, e := r.Load(id)
	if e != nil {
		return v, e
	}
	unlock, e := r.lock(ctx, v.Handle, v.ContestID, v.Index)
	if e != nil {
		return v, e
	}
	defer unlock()
	v, e = r.Load(id)
	if e != nil {
		return v, e
	}
	if v.State != "unknown" {
		return v, Fail("invalid_receipt", "Only an unknown receipt can be abandoned.")
	}
	v.State = "abandoned"
	return v, r.save(v)
}
