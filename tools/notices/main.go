// Collect license texts from the exact module versions in go.sum for binary distribution.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func main() {
	output := flag.String("output", "THIRD_PARTY_NOTICES.txt", "generated notice file")
	flag.Parse()
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	data, e := cmd.Output()
	if e != nil {
		panic(e)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	text.WriteString("Codeforces Client third-party module license notices\nGenerated from locked Go modules. Upstream CF Tool LICENSE is distributed separately.\n\n")
	goLicense, e := os.ReadFile(filepath.Join(runtime.GOROOT(), "LICENSE"))
	if e != nil {
		panic(e)
	}
	fmt.Fprintf(&text, "===== Go runtime / standard library (%s) =====\n%s\n", runtime.Version(), goLicense)
	for {
		var m struct {
			Path, Version, Dir string
			Main               bool
		}
		e = dec.Decode(&m)
		if e == io.EOF {
			break
		}
		if e != nil {
			panic(e)
		}
		if m.Main {
			continue
		}
		if m.Dir == "" {
			panic("Module has not been downloaded: " + m.Path)
		}
		entries, e := os.ReadDir(m.Dir)
		if e != nil {
			panic(e)
		}
		names := []string{}
		for _, v := range entries {
			n := strings.ToUpper(v.Name())
			if !v.IsDir() && (strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "COPYING") || n == "NOTICE") {
				names = append(names, v.Name())
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			panic("Missing license notice: " + m.Path)
		}
		fmt.Fprintf(&text, "===== %s %s =====\n", m.Path, m.Version)
		for _, n := range names {
			b, e := os.ReadFile(filepath.Join(m.Dir, n))
			if e != nil {
				panic(e)
			}
			fmt.Fprintf(&text, "--- %s ---\n%s\n", n, b)
		}
	}
	if e = os.WriteFile(*output, []byte(text.String()), 0644); e != nil {
		panic(e)
	}
}
