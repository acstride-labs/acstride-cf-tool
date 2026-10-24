package core

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type helperWorker struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	decoder *json.Decoder
}

func (w *helperWorker) close() {
	if w.cmd != nil {
		_ = w.in.Close()
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
		w.cmd = nil
	}
}

// PublicHelperPool reuses two anonymous transport processes and their curl sessions.
// Each process owns one request at a time; a timeout discards that worker's connection.
func PublicHelperPool(parent context.Context, executable string) (func(context.Context, string) (Page, error), func()) {
	pool := make(chan *helperWorker, 2)
	workers := []*helperWorker{{}, {}}
	for _, worker := range workers {
		pool <- worker
	}
	stop := func() {
		for range workers {
			worker := <-pool
			worker.close()
		}
	}
	read := func(ctx context.Context, requestPath string) (Page, error) {
		if !filepath.IsAbs(executable) || !regexp.MustCompile(`^/contest/[1-9][0-9]{0,8}/problem/[A-Z][0-9]{0,2}$`).MatchString(requestPath) {
			return Page{}, Fail("invalid_argument", "Invalid public helper request.")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		var worker *helperWorker
		select {
		case worker = <-pool:
		case <-ctx.Done():
			return Page{}, ctx.Err()
		}
		defer func() { pool <- worker }()
		if worker.cmd == nil {
			cmd := exec.CommandContext(parent, executable, "--serve")
			for _, value := range os.Environ() {
				key := strings.SplitN(value, "=", 2)[0]
				if !regexp.MustCompile(`(?i)^(PYTHON|PYINSTALLER_|_PYI_|IMPERSONATE_|CONDA|VIRTUAL_ENV|LD_|DYLD_|SSL_CERT_FILE|SSL_CERT_DIR|REQUESTS_CA_BUNDLE|CURL_CA_BUNDLE)`).MatchString(key) {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "PYINSTALLER_RESET_ENVIRONMENT=1")
			input, err := cmd.StdinPipe()
			if err != nil {
				return Page{}, err
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				_ = input.Close()
				return Page{}, err
			}
			cmd.Stderr = io.Discard
			if err = cmd.Start(); err != nil {
				_ = input.Close()
				_ = output.Close()
				return Page{}, Fail("helper_failed", "Cannot start public transport.")
			}
			worker.cmd, worker.in, worker.decoder = cmd, input, json.NewDecoder(output)
		}
		type response struct {
			raw json.RawMessage
			err error
		}
		done := make(chan response, 1)
		go func() {
			err := json.NewEncoder(worker.in).Encode(map[string]string{"path": requestPath})
			var raw json.RawMessage
			if err == nil {
				err = worker.decoder.Decode(&raw)
			}
			done <- response{raw, err}
		}()
		select {
		case <-ctx.Done():
			worker.close()
			<-done
			return Page{}, ctx.Err()
		case result := <-done:
			if result.err != nil {
				worker.close()
				return Page{}, Fail("helper_failed", "Public transport disconnected.")
			}
			return parsePublicHelper(result.raw, requestPath)
		}
	}
	return read, stop
}
