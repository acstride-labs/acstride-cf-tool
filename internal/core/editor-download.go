package core

import (
	"context"
	"net/url"
	"strconv"
	"sync"
)

// ContestProblems returns the problem list with at most one ranking row, never the full board.
func (c *Client) ContestProblems(ctx context.Context, id int) (Standings, error) {
	var result Standings
	err := c.API(ctx, "contest.standings", url.Values{"contestId": {strconv.Itoa(id)}, "from": {"1"}, "count": {"1"}}, &result)
	result.Rows = []map[string]any{}
	return result, err
}

type EditorEvent struct {
	Index   string         `json:"index"`
	Problem *EditorProblem `json:"problem,omitempty"`
	Error   *Error         `json:"error,omitempty"`
}

// DownloadEditorProblems runs the first text job alone, then two FIFO consumers.
// Images have a separate two-worker queue and never gate publication of text/samples.
func (c *Client) DownloadEditorProblems(ctx context.Context, id int, indices []string, assetsDir string, emit func(string, EditorEvent)) error {
	if id < 1 || id > 999999999 || len(indices) == 0 {
		return Fail("invalid_argument", "Specify a contest and problem indices.")
	}
	seen := map[string]bool{}
	for _, index := range indices {
		normalized, err := ProblemIndex(index)
		if err != nil || normalized != index || seen[index] {
			return Fail("invalid_argument", "Invalid or duplicate problem index.")
		}
		seen[index] = true
	}
	images := make(chan EditorProblem, len(indices))
	var imageWorkers sync.WaitGroup
	for range 2 {
		imageWorkers.Add(1)
		go func() {
			defer imageWorkers.Done()
			for problem := range images {
				updated, err := c.CacheProblemImages(ctx, problem, assetsDir)
				if err != nil {
					updated = problem
					updated.ImageFailures++
				}
				emit("assets", EditorEvent{Index: problem.Index, Problem: &updated})
			}
		}()
	}
	read := func(index string) {
		problem, err := c.ReadProblemText(ctx, id, index)
		if err != nil {
			emit("problemError", EditorEvent{Index: index, Error: Classify(err)})
			return
		}
		emit("problem", EditorEvent{Index: index, Problem: &problem})
		images <- problem
	}
	read(indices[0])
	jobs := make(chan string)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				read(index)
			}
		}()
	}
	for _, index := range indices[1:] {
		if ctx.Err() != nil {
			break
		}
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	close(images)
	imageWorkers.Wait()
	return ctx.Err()
}
