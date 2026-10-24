package cli

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/acstride-labs/acstride-cf-tool/internal/core"
)

type readRequest struct {
	RequestID string   `json:"requestId"`
	Command   string   `json:"command"`
	Method    string   `json:"method,omitempty"`
	ContestID int      `json:"contestId,omitempty"`
	Handle    string   `json:"handle,omitempty"`
	Count     int      `json:"count,omitempty"`
	Indices   []string `json:"indices,omitempty"`
	AssetsDir string   `json:"assetsDir,omitempty"`
}

// serve is an anonymous read-only NDJSON session. Login and submission are deliberately absent.
func (a *App) serve(parent context.Context, in io.Reader, out io.Writer, helper string) int {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if helper != "" {
		read, closePool := core.PublicHelperPool(ctx, helper)
		a.Client.ReadPublic = read
		defer closePool()
	}
	var lock sync.Mutex
	encoder := json.NewEncoder(out)
	send := func(id, kind string, data any, err error) {
		event := Envelope{Protocol: 1, RequestID: id, Type: kind, OK: err == nil, Data: data}
		if err != nil {
			event.Error = core.Classify(err)
		}
		lock.Lock()
		defer lock.Unlock()
		if encoder.Encode(event) != nil {
			cancel()
		}
	}
	if closer, ok := in.(io.Closer); ok {
		go func() { <-ctx.Done(); _ = closer.Close() }()
	}
	decoder := json.NewDecoder(in)
	slots := make(chan struct{}, 4)
	var requests sync.WaitGroup
	for {
		var request readRequest
		if err := decoder.Decode(&request); err != nil {
			cancel()
			requests.Wait()
			if err == io.EOF {
				return 0
			}
			return 2
		}
		if request.RequestID == "" || len(request.RequestID) > 64 {
			send(request.RequestID, "result", nil, core.Fail("invalid_argument", "Invalid request ID."))
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			requests.Wait()
			return 1
		}
		requests.Add(1)
		go func(r readRequest) {
			defer requests.Done()
			defer func() { <-slots }()
			deadline := 25 * time.Second
			if r.Command == "download" {
				deadline = 10 * time.Minute
			}
			requestCtx, stop := context.WithTimeout(ctx, deadline)
			defer stop()
			var data any
			var err error
			switch r.Command {
			case "capabilities":
				data = map[string]any{"protocol": 1, "anonymous": []string{"api", "problem.read", "contest.problems", "download.stream"}}
			case "contest.problems":
				if r.ContestID < 1 || r.ContestID > 999999999 {
					err = core.Fail("invalid_argument", "Invalid contest ID.")
				} else {
					data, err = a.Client.ContestProblems(requestCtx, r.ContestID)
				}
			case "download":
				err = a.Client.DownloadEditorProblems(requestCtx, r.ContestID, r.Indices, r.AssetsDir, func(kind string, event core.EditorEvent) { send(r.RequestID, kind, event, nil) })
			case "api":
				if r.Method != "contest.list" && r.Method != "contest.status" && r.Method != "contest.standings" {
					err = core.Fail("invalid_argument", "Unsupported read-only method.")
					break
				}
				if r.Method != "contest.list" && (r.ContestID < 1 || r.ContestID > 999999999) {
					err = core.Fail("invalid_argument", "Invalid contest ID.")
					break
				}
				if r.Method == "contest.status" {
					err = core.ValidHandle(r.Handle)
					if err != nil {
						break
					}
				}
				count := r.Count
				if count == 0 {
					count = 100
				}
				if count < 1 {
					err = core.Fail("invalid_argument", "Invalid count.")
					break
				}
				data, err = a.execute(requestCtx, options{contest: strconv.Itoa(r.ContestID), handle: r.Handle, count: count, from: 1, allRows: r.Method == "contest.standings"}, []string{"api", r.Method}, nil, nil)
			default:
				err = core.Fail("invalid_argument", "Unsupported read-only command.")
			}
			send(r.RequestID, "result", data, err)
		}(request)
	}
}
