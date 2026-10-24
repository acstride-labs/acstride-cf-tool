package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func Atomic(root *os.Root, name string, data []byte, mode os.FileMode) error {
	if info, e := root.Lstat(name); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return Fail("unsafe_path", "Refusing to replace a symbolic link.")
	}
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return e
	}
	tmp := name + "." + hex.EncodeToString(b) + ".tmp"
	f, e := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if e != nil {
		return e
	}
	defer root.Remove(tmp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return root.Rename(tmp, name)
}

type CachedProblem struct {
	Protocol  int       `json:"protocol"`
	FetchedAt time.Time `json:"fetchedAt"`
	Problem   Problem   `json:"problem"`
}

func (c *Client) Fetch(ctx context.Context, id int, index, output string, refresh bool) (CachedProblem, error) {
	index, e := ProblemIndex(index)
	if e != nil {
		return CachedProblem{}, e
	}
	var root *os.Root
	name := filepath.Join("contests", strconv.Itoa(id), ".acstride", "problems", index+".json")
	if output != "" {
		if e = os.MkdirAll(output, 0755); e != nil {
			return CachedProblem{}, e
		}
		root, e = os.OpenRoot(output)
		if e != nil {
			return CachedProblem{}, e
		}
		defer root.Close()
		if !refresh {
			if f, e := root.Open(name); e == nil {
				b, readErr := io.ReadAll(io.LimitReader(f, BodyLimit*3+1))
				f.Close()
				var cached CachedProblem
				if readErr == nil && len(b) <= BodyLimit*3 && json.Unmarshal(b, &cached) == nil && cached.Protocol == 1 && cached.Problem.ContestID == id && cached.Problem.Index == index && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < 24*time.Hour {
					// Re-sanitize cached HTML before exposing it to an editor.
					p, err := ParseProblem(Page{HTML: cached.Problem.StatementHTML, URL: Host + "/contest/" + strconv.Itoa(id) + "/problem/" + index, Status: 200}, id, index)
					if err == nil {
						cached.Problem = p
						return cached, nil
					}
				}
			}
		}
	}
	page, e := c.ProblemPage(ctx, id, index)
	if e != nil {
		return CachedProblem{}, e
	}
	problem, e := ParseProblem(page, id, index)
	if e != nil {
		return CachedProblem{}, e
	}
	cached := CachedProblem{Protocol: 1, FetchedAt: time.Now().UTC(), Problem: problem}
	if root != nil {
		if e = root.MkdirAll(filepath.Dir(name), 0755); e != nil {
			return CachedProblem{}, e
		}
		b, e := json.MarshalIndent(cached, "", "  ")
		if e != nil {
			return CachedProblem{}, e
		}
		if e = Atomic(root, name, b, 0644); e != nil {
			return CachedProblem{}, e
		}
	}
	return cached, nil
}

type ImportItem struct {
	Index   string         `json:"index"`
	Problem *CachedProblem `json:"problem,omitempty"`
	Error   *Error         `json:"error,omitempty"`
}

func (c *Client) Download(ctx context.Context, id int, output string, refresh bool, progress func(ImportItem)) ([]ImportItem, error) {
	if output == "" {
		return nil, Fail("invalid_argument", "A download output directory is required.")
	}
	board, e := c.Rankings(ctx, id, "", 1, 1)
	if e != nil {
		return nil, e
	}
	if board.Contest.Phase == "BEFORE" {
		return nil, Fail("problem_not_ready", "The contest has not started.")
	}
	if len(board.Problems) == 0 {
		return nil, Fail("problem_not_ready", "No released problems are available.")
	}
	result := []ImportItem{}
	failed := false
	for _, p := range board.Problems {
		if e = ctx.Err(); e != nil {
			return result, e
		}
		item := ImportItem{Index: p.Index}
		// Retry only safe reads and only explicitly transient failures.
		var value CachedProblem
		for attempt := 0; attempt < 3; attempt++ {
			value, e = c.Fetch(ctx, id, p.Index, output, refresh)
			if e == nil || !Classify(e).Retryable {
				break
			}
			if attempt < 2 {
				t := time.NewTimer(time.Duration(1<<attempt) * time.Second)
				select {
				case <-ctx.Done():
					t.Stop()
					return result, ctx.Err()
				case <-t.C:
				}
			}
		}
		if e != nil {
			item.Error = Classify(e)
			failed = true
		} else {
			item.Problem = &value
		}
		result = append(result, item)
		if progress != nil {
			progress(item)
		}
	}
	if failed {
		return result, Fail("partial_failure", "Some problems could not be downloaded. Successful caches and user files are preserved.")
	}
	return result, nil
}
func SourceHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
