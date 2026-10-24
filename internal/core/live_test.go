package core

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveAnonymous(t *testing.T) {
	if os.Getenv("ACSTRIDE_LIVE") != "1" {
		t.Skip("opt-in public network checks only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := New()
	contest, e := c.Contest(ctx, 1)
	if e != nil || contest.ID != 1 {
		t.Fatal(contest, e)
	}
	p, e := c.Fetch(ctx, 1, "A", t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Problem.Samples) != 1 || p.Problem.Samples[0].Input != "6 6 4\n" || p.Problem.Samples[0].Output != "4\n" {
		t.Fatal(p.Problem.Samples)
	}
	if _, e = c.Submissions(ctx, 1, "tourist", 1); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Rankings(ctx, 1, "", 1, 1); e != nil {
		t.Fatal(e)
	}
}
