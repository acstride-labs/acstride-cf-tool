package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Host = "https://codeforces.com"
const BodyLimit = 8_000_000

var verificationRE = regexp.MustCompile(`(?is)<title>\s*(?:just a moment|attention required)\b|id\s*=\s*["']challenge-form["']|(?:id|class)\s*=\s*["'][^"']*(?:cf-chl-|g-recaptcha)`)

func verification(body string) bool { return verificationRE.MatchString(body) }

type Page struct {
	HTML   string `json:"html"`
	URL    string `json:"url"`
	Status int    `json:"status"`
}
type Client struct {
	ReadPublic func(context.Context, string) (Page, error)
	HTTP       *http.Client
	base       string
	interval   time.Duration
	mu         sync.Mutex
	next       time.Time
}

func New() *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{base: Host, interval: 2100 * time.Millisecond}
	c.HTTP = &http.Client{Timeout: 25 * time.Second, Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return Fail("redirect_limit", "Too many redirects.")
		}
		if req.URL.Scheme != "https" || req.URL.Host != "codeforces.com" {
			return Fail("external_redirect", "An external redirect was rejected.")
		}
		// Never replay a credential or submission body on a 307/308 redirect.
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			return Fail("unsafe_redirect", "A request-body redirect was rejected.")
		}
		return nil
	}}
	return c
}
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := time.Until(c.next)
	if d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	c.next = time.Now().Add(c.interval)
	return nil
}
func (c *Client) Request(ctx context.Context, method, path string, form url.Values) (Page, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return Page{}, Fail("invalid_argument", "Invalid request path.")
	}
	if err := c.wait(ctx); err != nil {
		return Page{}, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", "ACStride-CF/0.2 (+https://github.com/acstride-labs/acstride-cf-tool)")
	req.Header.Set("Accept", "application/json,text/html;q=0.9")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", c.base)
		req.Header.Set("Referer", c.base+path)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, BodyLimit+1))
	if err != nil {
		return Page{}, err
	}
	if len(b) > BodyLimit {
		return Page{}, Fail("response_too_large", "Response exceeds the size limit.")
	}
	if verification(string(b)) {
		return Page{}, Fail("verification_required", "Codeforces requires site verification. Use the official website; no verification is bypassed.")
	}
	switch res.StatusCode {
	case 401:
		return Page{}, Fail("not_authenticated", "Sign in on the official website or explicitly configure an experimental session.")
	case 403:
		return Page{}, Fail("access_denied", "Codeforces refused this request.")
	case 404:
		return Page{}, Fail("not_found", "The resource is unavailable or not released yet.")
	case 429:
		return Page{}, &Error{Code: "rate_limited", Message: "Codeforces rate limit reached. Retry later.", Retryable: true}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Page{}, &Error{Code: "http_error", Message: "Codeforces returned an HTTP error.", Retryable: res.StatusCode >= 500}
	}
	return Page{HTML: string(b), URL: res.Request.URL.String(), Status: res.StatusCode}, nil
}
func (c *Client) API(ctx context.Context, method string, p url.Values, target any) error {
	page, err := c.Request(ctx, "GET", "/api/"+method+"?"+p.Encode(), nil)
	if err != nil {
		return err
	}
	var env struct {
		Status  string          `json:"status"`
		Result  json.RawMessage `json:"result"`
		Comment string          `json:"comment"`
	}
	if json.Unmarshal([]byte(page.HTML), &env) != nil {
		return Fail("invalid_response", "Codeforces returned an invalid API response.")
	}
	if env.Status != "OK" {
		comment := strings.ToLower(env.Comment)
		if strings.Contains(comment, "not started") {
			return Fail("problem_not_ready", "The contest has not started.")
		}
		if strings.Contains(comment, "not found") {
			return Fail("not_found", "The requested public resource was not found.")
		}
		if strings.Contains(comment, "limit exceeded") {
			return &Error{Code: "rate_limited", Message: "Codeforces API rate limit reached.", Retryable: true}
		}
		return Fail("api_rejected", "Codeforces rejected the request. Check the contest, handle and availability.")
	}
	if json.Unmarshal(env.Result, target) != nil {
		return Fail("invalid_response", "Codeforces returned an incompatible result.")
	}
	return nil
}
