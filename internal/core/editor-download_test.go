package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEditorFIFOFirstThenTwoAndImagesDoNotBlockText(t *testing.T) {
	c := New()
	c.interval = 0
	starts := make(chan string, 8)
	gates := map[string]chan struct{}{}
	for _, index := range []string{"A", "B", "C", "D", "E"} {
		gates[index] = make(chan struct{})
	}
	c.ReadPublic = func(ctx context.Context, p string) (Page, error) {
		index := p[strings.LastIndex(p, "/")+1:]
		starts <- index
		select {
		case <-gates[index]:
		case <-ctx.Done():
			return Page{}, ctx.Err()
		}
		return Page{Status: 200, URL: Host + p, HTML: `<div class="problem-statement"><div class="title">` + index + `. Test</div><img src="https://codeforces.org/test.png"><p>text</p></div>`}, nil
	}
	images := make(chan struct{})
	c.HTTP.Transport = imageTransport(func(req *http.Request) (*http.Response, error) {
		select {
		case <-images:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("png"))}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	bodies, assets := 0, 0
	done := make(chan error, 1)
	go func() {
		done <- c.DownloadEditorProblems(ctx, 1, []string{"A", "B", "C", "D", "E"}, t.TempDir(), func(kind string, event EditorEvent) {
			mu.Lock()
			defer mu.Unlock()
			if kind == "problem" {
				bodies++
			}
			if kind == "assets" {
				assets++
			}
		})
	}()
	next := func() string {
		select {
		case index := <-starts:
			return index
		case <-ctx.Done():
			t.Fatal("job did not start")
			return ""
		}
	}
	if got := next(); got != "A" {
		t.Fatal(got)
	}
	select {
	case index := <-starts:
		t.Fatalf("started %s before first problem finished", index)
	case <-time.After(30 * time.Millisecond):
	}
	close(gates["A"])
	second, third := next(), next()
	if !((second == "B" && third == "C") || (second == "C" && third == "B")) {
		t.Fatalf("%s %s", second, third)
	}
	select {
	case index := <-starts:
		t.Fatalf("more than two consumers: %s", index)
	case <-time.After(30 * time.Millisecond):
	}
	close(gates["B"])
	if got := next(); got != "D" {
		t.Fatal(got)
	}
	close(gates["C"])
	if got := next(); got != "E" {
		t.Fatal(got)
	}
	close(gates["D"])
	close(gates["E"])
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n, a := bodies, assets
		mu.Unlock()
		if n == 5 {
			if a != 0 {
				t.Fatal("images unexpectedly completed")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("text waited for images")
		}
		time.Sleep(time.Millisecond)
	}
	close(images)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if bodies != 5 || assets != 5 {
		t.Fatalf("bodies=%d assets=%d", bodies, assets)
	}
}
func TestContestProblemDiscoveryRequestsOnlyOneRow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("count") != "1" || r.URL.Query().Get("from") != "1" {
			t.Error(r.URL.String())
		}
		_, _ = io.WriteString(w, `{"status":"OK","result":{"contest":{"id":1},"problems":[{"index":"A","name":"test"}],"rows":[{"rank":1}]}}`)
	}))
	defer server.Close()
	c := New()
	c.base = server.URL
	c.interval = 0
	result, err := c.ContestProblems(context.Background(), 1)
	if err != nil || len(result.Problems) != 1 || len(result.Rows) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}
