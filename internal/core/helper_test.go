package core

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ACSTRIDE_TEST_HELPER") != "" {
		if len(os.Args) > 1 && os.Args[1] == "--serve" {
			decoder := json.NewDecoder(os.Stdin)
			for {
				var request struct {
					Path string `json:"path"`
				}
				if decoder.Decode(&request) != nil {
					os.Exit(0)
				}
				json.NewEncoder(os.Stdout).Encode(Page{HTML: statement + strconv.Itoa(os.Getpid()), URL: Host + request.Path, Status: 200})
			}
		}

		var v struct {
			Path string `json:"path"`
		}
		json.NewDecoder(os.Stdin).Decode(&v)
		u := Host + v.Path
		if os.Getenv("ACSTRIDE_TEST_HELPER") == "external" {
			u = "https://evil.test/"
		}
		body := statement
		if os.Getenv("ACSTRIDE_TEST_HELPER") == "large" {
			body += strings.Repeat("x", 8_000_001)
		}
		json.NewEncoder(os.Stdout).Encode(Page{HTML: body, URL: u, Status: 200})
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

func TestPublicHelperPoolReusesProcesses(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACSTRIDE_TEST_HELPER", "good")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read, closePool := PublicHelperPool(ctx, executable)
	defer closePool()
	first, err := read(ctx, "/contest/1/problem/A")
	if err != nil {
		t.Fatal(err)
	}
	second, err := read(ctx, "/contest/1/problem/B")
	if err != nil {
		t.Fatal(err)
	}
	third, err := read(ctx, "/contest/1/problem/C")
	if err != nil {
		t.Fatal(err)
	}
	if first.HTML != third.HTML || first.HTML == second.HTML {
		t.Fatal("transport processes were not reused")
	}
	_, err = read(ctx, "/enter")
	code(t, err, "invalid_argument")
}
