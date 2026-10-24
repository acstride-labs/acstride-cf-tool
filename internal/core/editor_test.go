package core

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type imageTransport func(*http.Request) (*http.Response, error)

func (f imageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEditorReadOwnsAssetsAndPreservesSampleWhitespace(t *testing.T) {
	c := New()
	c.interval = 0
	c.ReadPublic = func(context.Context, string) (Page, error) {
		return Page{Status: 200, URL: Host + "/contest/1/problem/A", HTML: `<div class="problem-statement"><div class="title">A. 中文</div><p>$$$a+b$$$</p><img src="https://espresso.codeforces.com/p.png"><img src="https://espresso.codeforces.com/p.png"><img src="https://codeforces.org/no.svg"><img src="https://evil.test/x.png" onerror="bad()"><div class="sample-test"><div class="input"><pre><div class="test-example-line">你好</div><div class="test-example-line"></div><div class="test-example-line">2 3</div></pre></div><div class="output"><pre></pre></div></div></div>`}, nil
	}
	count := 0
	c.HTTP.Transport = imageTransport(func(r *http.Request) (*http.Response, error) {
		count++
		if r.Header.Get("Cookie") != "" {
			t.Fatal("image request attached session")
		}
		kind := "image/png"
		if strings.HasSuffix(r.URL.Path, ".svg") {
			kind = "image/svg+xml"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {kind}}, Body: io.NopCloser(strings.NewReader("image-bytes")), Request: r}, nil
	})
	folder := t.TempDir()
	p, e := c.ReadProblem(context.Background(), 1, "A", folder)
	if e != nil {
		t.Fatal(e)
	}
	if p.Samples[0].Input != "你好\n\n2 3\n" || p.Samples[0].Output != "\n" {
		t.Fatalf("%+v", p.Samples)
	}
	if count != 2 || p.ImageFailures != 1 || len(p.Assets) != 1 {
		t.Fatalf("calls=%d result=%+v", count, p)
	}
	if !strings.Contains(p.StatementHTML, "acstride-asset:"+p.Assets[0]) || strings.Contains(p.StatementHTML, "evil.test") || !strings.Contains(p.StatementHTML, "$$$a+b$$$") {
		t.Fatal(p.StatementHTML)
	}
	files, _ := os.ReadDir(folder)
	if len(files) != 1 {
		t.Fatal(files)
	}
	if _, e = c.ReadProblem(context.Background(), 0, "A", folder); e == nil {
		t.Fatal("invalid contest accepted")
	}
}

func TestImagePolicy(t *testing.T) {
	c := New()
	c.interval = 0
	for _, address := range []string{"http://codeforces.com/x", "https://codeforces.com.evil.test/x", "https://user:password@codeforces.com/x", "https://codeforces.com:444/x", "https://evil.test/x"} {
		if _, _, e := c.image(context.Background(), address); e == nil {
			t.Fatal(address)
		}
	}
	for _, tc := range []struct {
		status     int
		kind, body string
	}{{302, "image/png", "x"}, {200, "image/svg+xml", "x"}, {200, "image/png", strings.Repeat("x", 5000001)}} {
		c.HTTP.Transport = imageTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {tc.kind}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
		})
		if _, _, e := c.image(context.Background(), Host+"/x"); e == nil {
			t.Fatal("unsafe image accepted")
		}
	}
}

func TestSharedPacingAndCancellation(t *testing.T) {
	directory := t.TempDir()
	first := SharedPacer(directory, 50*time.Millisecond)
	second := SharedPacer(directory, 50*time.Millisecond)
	if e := first(context.Background()); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if e := second(ctx); e == nil {
		t.Fatal("cancel ignored")
	}
	var wg sync.WaitGroup
	for _, pace := range []func(context.Context) error{first, second} {
		wg.Add(1)
		go func(p func(context.Context) error) {
			defer wg.Done()
			if e := p(context.Background()); e != nil {
				t.Error(e)
			}
		}(pace)
	}
	wg.Wait()
	if time.Since(start) < 90*time.Millisecond {
		t.Fatal("independent clients bypassed shared pacing")
	}
	if _, e := os.Stat(filepath.Join(directory, "last-start")); e != nil {
		t.Fatal(e)
	}
}
