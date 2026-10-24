package client

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/acstride-labs/acstride-cf-tool/internal/core"
	"github.com/acstride-labs/acstride-cf-tool/util"
)

func findSample(body []byte) (input, output [][]byte, err error) {
	p, e := core.ParseProblem(core.Page{HTML: string(body)}, 0, "")
	if e != nil {
		return nil, nil, e
	}
	for _, s := range p.Samples {
		input = append(input, []byte(s.Input))
		output = append(output, []byte(s.Output))
	}
	return input, output, nil
}
func (c *Client) ParseProblem(URL, path string, mu *sync.Mutex) (samples int, standardIO bool, err error) {
	body, err := util.GetBody(c.client, URL)
	if err != nil {
		return 0, false, err
	}
	input, output, err := findSample(body)
	if err != nil {
		return 0, false, err
	}
	// This legacy command exports official samples only, with exclusive creation.
	// Existing sample files are never overwritten. The modern command uses a separate cache.
	for i := range input {
		for _, item := range []struct {
			name string
			data []byte
		}{{fmt.Sprintf("in%d.txt", i+1), input[i]}, {fmt.Sprintf("ans%d.txt", i+1), output[i]}} {
			f, e := os.OpenFile(filepath.Join(path, item.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if os.IsExist(e) {
				continue
			}
			if e != nil {
				return i, false, e
			}
			_, e = f.Write(item.data)
			closeErr := f.Close()
			if e != nil {
				return i, false, e
			}
			if closeErr != nil {
				return i, false, closeErr
			}
		}
	}
	return len(input), true, nil
}
func (c *Client) Parse(info Info) (problems, paths []string, err error) {
	id := info.ProblemID
	if id == "" {
		stats, e := c.Statis(info)
		if e != nil {
			return nil, nil, e
		}
		for _, v := range stats {
			problems = append(problems, v.ID)
		}
	} else {
		problems = []string{id}
	}
	for i, id := range problems {
		if i > 0 {
			time.Sleep(2100 * time.Millisecond)
		}
		index, e := core.ProblemIndex(id)
		if e != nil {
			return problems, paths, e
		}
		info.ProblemID = index
		u, e := info.ProblemURL(c.host)
		if e != nil {
			return problems, paths, e
		}
		path := filepath.Join(info.RootPath, info.ContestID, index)
		if e = os.MkdirAll(path, 0755); e != nil {
			return problems, paths, e
		}
		paths = append(paths, path)
		_, _, e = c.ParseProblem(u, path, nil)
		if e != nil {
			return problems, paths, e
		}
	}
	return
}
