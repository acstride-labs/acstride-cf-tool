package core

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var indexRE = regexp.MustCompile(`^[A-Z][0-9]{0,2}$`)
var handleRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,24}$`)

type Contest struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Type     string `json:"type,omitempty"`
	Frozen   bool   `json:"frozen,omitempty"`
	Duration int    `json:"durationSeconds"`
	Start    int64  `json:"startTimeSeconds"`
}
type ProblemRef struct {
	ContestID int    `json:"contestId"`
	Index     string `json:"index"`
	Name      string `json:"name"`
}
type Submission struct {
	ID        int64      `json:"id"`
	ContestID int        `json:"contestId"`
	Time      int64      `json:"creationTimeSeconds"`
	Problem   ProblemRef `json:"problem"`
	Language  string     `json:"programmingLanguage"`
	Verdict   string     `json:"verdict,omitempty"`
	Testset   string     `json:"testset,omitempty"`
	Passed    int        `json:"passedTestCount"`
	Millis    int        `json:"timeConsumedMillis"`
	Memory    int64      `json:"memoryConsumedBytes"`
	Author    struct {
		Members []struct {
			Handle string `json:"handle"`
		} `json:"members"`
	} `json:"author"`
}
type Standings struct {
	Contest  Contest          `json:"contest"`
	Problems []ProblemRef     `json:"problems"`
	Rows     []map[string]any `json:"rows"`
}

func ContestID(s string) (int, error) {
	if strings.HasPrefix(s, "https://codeforces.com/contest/") {
		u, e := url.Parse(s)
		if e != nil || u.Host != "codeforces.com" || u.RawQuery != "" || u.Fragment != "" {
			return 0, Fail("invalid_argument", "Use a regular contest ID or URL.")
		}
		s = strings.TrimSuffix(strings.TrimPrefix(u.Path, "/contest/"), "/")
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || n > 999999999 {
		return 0, Fail("invalid_argument", "Invalid contest ID.")
	}
	return n, nil
}
func ProblemIndex(s string) (string, error) {
	s = strings.ToUpper(s)
	if !indexRE.MatchString(s) {
		return "", Fail("invalid_argument", "Invalid problem index.")
	}
	return s, nil
}
func ValidHandle(s string) error {
	if !handleRE.MatchString(s) {
		return Fail("invalid_argument", "Invalid Codeforces handle.")
	}
	return nil
}
func (c *Client) Contest(ctx context.Context, id int) (Contest, error) {
	var contests []Contest
	if e := c.API(ctx, "contest.list", url.Values{"gym": {"false"}}, &contests); e != nil {
		return Contest{}, e
	}
	for _, v := range contests {
		if v.ID == id {
			return v, nil
		}
	}
	return Contest{}, Fail("not_found", "Contest not found in the public contest list.")
}
func (c *Client) Rankings(ctx context.Context, id int, handle string, from, count int) (Standings, error) {
	if from < 1 || count < 1 || count > 100 {
		return Standings{}, Fail("invalid_argument", "Invalid ranking page.")
	}
	p := url.Values{"contestId": {strconv.Itoa(id)}}
	if handle != "" {
		if e := ValidHandle(handle); e != nil {
			return Standings{}, e
		}
	}
	var result Standings
	err := c.API(ctx, "contest.standings", p, &result)
	if err != nil {
		return result, err
	}
	// The public endpoint currently rejects some filtered query combinations.
	// Use the known-compatible contestId-only request and page/filter locally.
	rows := []map[string]any{}
	for _, row := range result.Rows {
		match := handle == ""
		if party, ok := row["party"].(map[string]any); ok {
			if members, ok := party["members"].([]any); ok {
				for _, member := range members {
					if m, ok := member.(map[string]any); ok {
						if h, ok := m["handle"].(string); ok && strings.EqualFold(h, handle) {
							match = true
						}
					}
				}
			}
		}
		if match {
			rows = append(rows, row)
		}
	}
	start := min(from-1, len(rows))
	end := min(start+count, len(rows))
	result.Rows = rows[start:end]
	return result, err
}
func (c *Client) Submissions(ctx context.Context, id int, handle string, count int) ([]Submission, error) {
	if e := ValidHandle(handle); e != nil {
		return nil, e
	}
	method := "user.status"
	p := url.Values{"handle": {handle}, "from": {"1"}, "count": {strconv.Itoa(count)}}
	if id > 0 {
		method = "contest.status"
		p.Set("contestId", strconv.Itoa(id))
	}
	result := []Submission{}
	err := c.API(ctx, method, p, &result)
	return result, err
}
func (c *Client) ProblemPage(ctx context.Context, id int, index string) (Page, error) {
	index, e := ProblemIndex(index)
	if e != nil {
		return Page{}, e
	}
	path := "/contest/" + strconv.Itoa(id) + "/problem/" + index
	var p Page
	if c.ReadPublic != nil {
		if e = c.wait(ctx); e != nil {
			return Page{}, e
		}
		p, e = c.ReadPublic(ctx, path)
	} else {
		p, e = c.Request(ctx, "GET", path, nil)
	}
	if e != nil {
		return Page{}, e
	}
	if p.URL != c.base+path {
		return Page{}, Fail("unexpected_redirect", "Problem page redirected to another page.")
	}
	return p, nil
}
