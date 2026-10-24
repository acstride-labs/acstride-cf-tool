package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLargeRankingsAndUnrestrictedPageCount(t *testing.T) {
	// More than the former 8 MB transport limit, with more than 100 rows.
	board := Standings{Contest: Contest{ID: 1, Name: strings.Repeat("x", 8_000_001)}}
	for i := 1; i <= 150; i++ {
		board.Rows = append(board.Rows, map[string]any{"rank": i})
	}
	body, err := json.Marshal(map[string]any{"status": "OK", "result": board})
	if err != nil {
		t.Fatal(err)
	}
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	got, err := c.Rankings(context.Background(), 1, "", 1, 150)
	if err != nil || len(got.Rows) != 150 || len(got.Contest.Name) != 8_000_001 {
		t.Fatalf("rows=%d error=%v", len(got.Rows), err)
	}
	// Huge positive offsets/counts must not overflow local slicing.
	got, err = c.Rankings(context.Background(), 1, "", int(^uint(0)>>1), int(^uint(0)>>1))
	if err != nil || len(got.Rows) != 0 {
		t.Fatalf("offset: rows=%d error=%v", len(got.Rows), err)
	}
}

func TestLargeImagesAndMoreThanFiftyAssets(t *testing.T) {
	c := New()
	c.interval = 0
	image := strings.Repeat("x", 5_000_001)
	c.HTTP.Transport = imageTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: int64(len(image)), Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader(image)), Request: r}, nil
	})
	bytes, ext, err := c.image(context.Background(), Host+"/big.png")
	if err != nil || ext != "png" || len(bytes) != len(image) {
		t.Fatal(ext, err, len(bytes))
	}
	html := `<div class="problem-statement"><div class="title">A</div>`
	for i := 0; i < 60; i++ {
		html += fmt.Sprintf(`<img src="https://codeforces.org/image-%d.png">`, i)
	}
	html += `</div>`
	c.ReadPublic = func(context.Context, string) (Page, error) {
		return Page{URL: Host + "/contest/1/problem/A", HTML: html, Status: 200}, nil
	}
	// Keep the image-count check small, independently of the large-image check.
	c.HTTP.Transport = imageTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("small")), Request: r}, nil
	})
	result, err := c.ReadProblem(context.Background(), 1, "A", t.TempDir())
	if err != nil || result.ImageFailures != 0 || strings.Count(result.StatementHTML, "acstride-asset:") != 60 {
		t.Fatal(result.ImageFailures, err)
	}
}

func TestLargeHelperPage(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEFORCES_CLIENT_TEST_HELPER", "large")
	page, err := ExternalPublicHelper(executable)(context.Background(), "/contest/1/problem/A")
	if err != nil || len(page.HTML) <= 8_000_000 {
		t.Fatal(len(page.HTML), err)
	}
}
