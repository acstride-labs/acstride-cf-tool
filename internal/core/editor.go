package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func officialURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	h := u.Hostname()
	return h == "codeforces.com" || strings.HasSuffix(h, ".codeforces.com") || h == "codeforces.org" || strings.HasSuffix(h, ".codeforces.org")
}

type EditorProblem struct {
	Problem
	FetchedAt     time.Time `json:"fetchedAt"`
	ImageFailures int       `json:"imageFailures"`
	Assets        []string  `json:"assets"`
}

// ReadProblem owns parsing and asset transport. It writes only content-addressed
// image files in the explicit asset root, never solutions or custom tests.
func (c *Client) ReadProblemText(ctx context.Context, id int, index string) (EditorProblem, error) {
	if id < 1 || id > 999999999 {
		return EditorProblem{}, Fail("invalid_argument", "Invalid contest ID.")
	}
	page, err := c.ProblemPage(ctx, id, index)
	if err != nil {
		return EditorProblem{}, err
	}
	index, _ = ProblemIndex(index)
	p, err := ParseProblem(page, id, index)
	if err != nil {
		return EditorProblem{}, err
	}
	result := EditorProblem{Problem: p, FetchedAt: time.Now().UTC(), Assets: []string{}}
	return result, nil
}

// ReadProblem retains the synchronous CLI API; editor streams use the two phases directly.
func (c *Client) ReadProblem(ctx context.Context, id int, index, assetsDir string) (EditorProblem, error) {
	result, err := c.ReadProblemText(ctx, id, index)
	if err != nil || assetsDir == "" {
		return result, err
	}
	return c.CacheProblemImages(ctx, result, assetsDir)
}

func (c *Client) CacheProblemImages(ctx context.Context, result EditorProblem, assetsDir string) (EditorProblem, error) {
	var err error
	if assetsDir == "" {
		return result, nil
	}
	if err = os.MkdirAll(assetsDir, 0755); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(assetsDir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	doc, err := document(result.StatementHTML)
	if err != nil {
		return result, err
	}
	images := all(doc, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "img" && attr(n, "src") != "" })
	cached := map[string]string{}
	for _, image := range images {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		source := attr(image, "src")
		name := cached[source]
		if name == "" {
			var data []byte
			var ext string
			data, ext, err = c.image(ctx, source)
			if err == nil {
				hash := sha256.Sum256(data)
				name = hex.EncodeToString(hash[:]) + "." + ext
				err = Atomic(root, name, data, 0644)
				if err == nil {
					cached[source] = name
					result.Assets = append(result.Assets, name)
				} else {
					name = ""
				}
			}
		}
		if name == "" {
			result.ImageFailures++
			continue
		}
		for j := range image.Attr {
			if image.Attr[j].Key == "src" {
				image.Attr[j].Val = "acstride-asset:" + name
			}
		}
	}
	var b bytes.Buffer
	statement := withClass(doc, "problem-statement")
	if statement == nil {
		return result, Fail("parse_error", "Missing statement.")
	}
	if err = html.Render(&b, statement); err != nil {
		return result, err
	}
	result.StatementHTML = b.String()
	return result, nil
}

func (c *Client) image(ctx context.Context, source string) ([]byte, string, error) {
	u, err := url.Parse(source)
	if err != nil || !officialURL(u) {
		return nil, "", Fail("invalid_argument", "Unsupported image origin.")
	}
	if err = c.wait(ctx); err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Codeforces-Client/0.3")
	// Never attach account cookies or follow redirects while reading images.
	transport := *c.HTTP
	transport.Jar = nil
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := transport.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}[strings.ToLower(strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0]))]
	if res.StatusCode != 200 || ext == "" {
		return nil, "", Fail("invalid_response", "Image is unavailable or unsupported.")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	return data, ext, nil
}
