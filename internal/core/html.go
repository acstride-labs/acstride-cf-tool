package core

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type Sample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}
type Problem struct {
	ContestID     int      `json:"contestId"`
	Index         string   `json:"index"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	StatementHTML string   `json:"statementHtml"`
	StatementText string   `json:"statementText"`
	TimeLimit     string   `json:"timeLimit"`
	MemoryLimit   string   `json:"memoryLimit"`
	Samples       []Sample `json:"samples"`
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
func class(n *html.Node, value string) bool {
	for _, s := range strings.Fields(attr(n, "class")) {
		if s == value {
			return true
		}
	}
	return false
}
func find(n *html.Node, p func(*html.Node) bool) *html.Node {
	if p(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if v := find(child, p); v != nil {
			return v
		}
	}
	return nil
}
func all(n *html.Node, p func(*html.Node) bool) (nodes []*html.Node) {
	if p(n) {
		nodes = append(nodes, n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		nodes = append(nodes, all(child, p)...)
	}
	return
}
func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Data == "script" || n.Data == "style" {
		return ""
	}
	var b strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(text(child))
	}
	if n.Data == "br" || n.Data == "div" || n.Data == "p" || n.Data == "li" || n.Data == "tr" {
		b.WriteByte('\n')
	}
	return b.String()
}
func withClass(n *html.Node, s string) *html.Node {
	return find(n, func(n *html.Node) bool { return class(n, s) })
}
func document(body string) (*html.Node, error) {
	n, e := html.Parse(strings.NewReader(body))
	if e != nil {
		return nil, Fail("parse_error", "Cannot parse Codeforces HTML.")
	}
	return n, nil
}

var tags = map[string]bool{"div": true, "span": true, "p": true, "pre": true, "code": true, "br": true, "b": true, "strong": true, "i": true, "em": true, "sub": true, "sup": true, "ul": true, "ol": true, "li": true, "table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true, "a": true, "img": true, "h1": true, "h2": true, "h3": true, "h4": true, "blockquote": true}

// Treat downloaded HTML as data: remove executable elements and all active attributes.
func sanitize(n *html.Node) *html.Node {
	if n.Type == html.TextNode {
		return &html.Node{Type: html.TextNode, Data: n.Data}
	}
	if n.Type != html.ElementNode || !tags[n.Data] {
		return nil
	}
	out := &html.Node{Type: html.ElementNode, Data: n.Data, DataAtom: n.DataAtom}
	for _, a := range n.Attr {
		if a.Key == "class" || a.Key == "title" || a.Key == "alt" {
			out.Attr = append(out.Attr, a)
		}
		if (n.Data == "a" && a.Key == "href") || (n.Data == "img" && a.Key == "src") {
			u, e := url.Parse(a.Val)
			if e != nil {
				continue
			}
			base, _ := url.Parse(Host)
			u = base.ResolveReference(u)
			if u.Scheme == "https" && u.Host == "codeforces.com" && u.User == nil {
				out.Attr = append(out.Attr, html.Attribute{Key: a.Key, Val: u.String()})
			}
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if safe := sanitize(ch); safe != nil {
			out.AppendChild(safe)
		}
	}
	return out
}
func ParseProblem(page Page, id int, index string) (Problem, error) {
	doc, e := document(page.HTML)
	if e != nil {
		return Problem{}, e
	}
	node := withClass(doc, "problem-statement")
	if node == nil {
		return Problem{}, Fail("problem_not_ready", "The statement is missing, unreleased, or its format changed.")
	}
	title := strings.TrimSpace(text(withClass(node, "title")))
	if title == "" {
		return Problem{}, Fail("parse_error", "Problem title is missing.")
	}
	ins := all(node, func(n *html.Node) bool { return class(n, "input") && n.Parent != nil && class(n.Parent, "sample-test") })
	outs := all(node, func(n *html.Node) bool {
		return class(n, "output") && n.Parent != nil && class(n.Parent, "sample-test")
	})
	if len(ins) != len(outs) {
		return Problem{}, Fail("parse_error", "Sample inputs and outputs do not match.")
	}
	samples := []Sample{}
	for i := range ins {
		a := find(ins[i], func(n *html.Node) bool { return n.Data == "pre" })
		b := find(outs[i], func(n *html.Node) bool { return n.Data == "pre" })
		if a == nil || b == nil {
			return Problem{}, Fail("parse_error", "A sample is incomplete.")
		}
		samples = append(samples, Sample{Input: strings.Trim(text(a), "\r\n") + "\n", Output: strings.Trim(text(b), "\r\n") + "\n"})
	}
	var rendered bytes.Buffer
	if e = html.Render(&rendered, sanitize(node)); e != nil {
		return Problem{}, e
	}
	return Problem{ContestID: id, Index: index, Name: title, URL: page.URL, StatementHTML: rendered.String(), StatementText: strings.TrimSpace(text(node)), TimeLimit: strings.TrimSpace(text(withClass(node, "time-limit"))), MemoryLimit: strings.TrimSpace(text(withClass(node, "memory-limit"))), Samples: samples}, nil
}

type Language struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func ParseLanguages(body string) ([]Language, error) {
	doc, e := document(body)
	if e != nil {
		return nil, e
	}
	sel := find(doc, func(n *html.Node) bool { return n.Data == "select" && attr(n, "name") == "programTypeId" })
	if sel == nil {
		return nil, Fail("not_authenticated", "Submission languages are unavailable. Check the experimental session.")
	}
	result := []Language{}
	for _, n := range all(sel, func(n *html.Node) bool { return n.Data == "option" }) {
		id := attr(n, "value")
		if regexp.MustCompile(`^[0-9]+$`).MatchString(id) && !hasAttr(n, "disabled") {
			result = append(result, Language{ID: id, Name: strings.TrimSpace(text(n))})
		}
	}
	if len(result) == 0 {
		return nil, Fail("parse_error", "No submission languages found.")
	}
	return result, nil
}
func CSRF(body string) (string, error) {
	doc, e := document(body)
	if e != nil {
		return "", e
	}
	node := find(doc, func(n *html.Node) bool { return n.Data == "input" && attr(n, "name") == "csrf_token" })
	if node != nil && attr(node, "value") != "" {
		return attr(node, "value"), nil
	}
	node = find(doc, func(n *html.Node) bool {
		return n.Data == "meta" && (strings.EqualFold(attr(n, "name"), "X-Csrf-Token") || strings.EqualFold(attr(n, "name"), "csrf-token"))
	})
	if node != nil && attr(node, "content") != "" {
		return attr(node, "content"), nil
	}
	re := regexp.MustCompile(`\bcsrf\s*[:=]\s*['"]([A-Za-z0-9_-]{8,256})['"]`)
	m := re.FindStringSubmatch(body)
	if len(m) > 1 {
		return m[1], nil
	}
	return "", Fail("parse_error", "The website CSRF token could not be read.")
}
func Handle(body string) (string, error) {
	doc, e := document(body)
	if e != nil {
		return "", e
	}
	// Require the logout control as well as a profile link, not a random page's author.
	logout := find(doc, func(n *html.Node) bool { return n.Data == "a" && strings.HasPrefix(attr(n, "href"), "/logout") })
	if logout == nil {
		return "", Fail("not_authenticated", "The session is not authenticated.")
	}
	re := regexp.MustCompile(`\bhandle\s*=\s*['"]([A-Za-z0-9_.-]{1,24})['"]`)
	if m := re.FindStringSubmatch(body); len(m) > 1 {
		return m[1], nil
	}
	header := withClass(doc, "lang-chooser")
	if header == nil {
		return "", Fail("parse_error", "Cannot identify the authenticated handle.")
	}
	link := find(header, func(n *html.Node) bool { return n.Data == "a" && strings.HasPrefix(attr(n, "href"), "/profile/") })
	if link != nil {
		h := strings.TrimPrefix(attr(link, "href"), "/profile/")
		if ValidHandle(h) == nil {
			return h, nil
		}
	}
	return "", Fail("parse_error", "Cannot identify the authenticated handle.")
}
