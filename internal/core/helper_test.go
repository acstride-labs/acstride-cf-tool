package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ACSTRIDE_TEST_HELPER") != "" {
		var v struct {
			Path string `json:"path"`
		}
		json.NewDecoder(os.Stdin).Decode(&v)
		u := Host + v.Path
		if os.Getenv("ACSTRIDE_TEST_HELPER") == "external" {
			u = "https://evil.test/"
		}
		json.NewEncoder(os.Stdout).Encode(Page{HTML: statement, URL: u, Status: 200})
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestAnonymousHelperAdapter(t *testing.T) {
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("ACSTRIDE_TEST_HELPER", "good")
	read := ExternalPublicHelper(executable)
	p, e := read(context.Background(), "/contest/1/problem/A")
	if e != nil || p.Status != 200 {
		t.Fatal(e)
	}
	_, e = read(context.Background(), "/enter")
	code(t, e, "invalid_argument")
	t.Setenv("ACSTRIDE_TEST_HELPER", "external")
	_, e = read(context.Background(), "/contest/1/problem/A")
	code(t, e, "unexpected_redirect")
}
