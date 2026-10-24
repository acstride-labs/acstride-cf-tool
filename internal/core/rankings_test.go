package core

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestRankingsUsesCompatibleQueryAndFiltersLocally(t *testing.T) {
	c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "contestId=1" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"status":"OK","result":{"contest":{"id":1},"problems":[],"rows":[{"rank":1,"party":{"members":[{"handle":"first"}]}},{"rank":2,"party":{"members":[{"handle":"Tester"}]}}]}}`)
	})
	v, e := c.Rankings(context.Background(), 1, "tester", 1, 100)
	if e != nil || len(v.Rows) != 1 || v.Rows[0]["rank"] != float64(2) {
		t.Fatal(v, e)
	}
	v, e = c.Rankings(context.Background(), 1, "", 2, 1)
	if e != nil || len(v.Rows) != 1 || v.Rows[0]["rank"] != float64(2) {
		t.Fatal(v, e)
	}
}
