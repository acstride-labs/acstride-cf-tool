package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/acstride-labs/acstride-cf-tool/config"
	"github.com/acstride-labs/acstride-cf-tool/util"
	"github.com/fatih/color"
	ansi "github.com/k0kubun/go-ansi"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/shirou/gopsutil/process"
)

func splitCmd(s string) (res []string) {
	// https://github.com/vrischmann/shlex/blob/master/shlex.go
	var buf bytes.Buffer
	insideQuotes := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) && !insideQuotes:
			if buf.Len() > 0 {
				res = append(res, buf.String())
				buf.Reset()
			}
		case r == '"' || r == '\'':
			if insideQuotes {
				res = append(res, buf.String())
				buf.Reset()
				insideQuotes = false
				continue
			}
			insideQuotes = true
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		res = append(res, buf.String())
	}
	return
}

func plain(raw []byte) string {
	buf := bufio.NewScanner(bytes.NewReader(raw))
	var b bytes.Buffer
	newline := []byte{'\n'}
	for buf.Scan() {
		b.Write(bytes.TrimSpace(buf.Bytes()))
		b.Write(newline)
	}
	return b.String()
}

func judge(sampleID, command string) error {
	inPath := fmt.Sprintf("in%v.txt", sampleID)
	ansPath := fmt.Sprintf("ans%v.txt", sampleID)
	input, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer input.Close()
	var o bytes.Buffer
	output := io.Writer(&o)

	cmds := splitCmd(command)
	if len(cmds) == 0 {
		return errors.New("Empty execution command")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
	cmd.WaitDelay = time.Second
	cmd.Stdin = input
	cmd.Stdout = &limitedOutput{writer: output, remaining: 1_000_000}
	cmd.Stderr = &limitedOutput{writer: os.Stderr, remaining: 1_000_000}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Runtime Error #%v ... %v", sampleID, err.Error())
	}

	pid := int32(cmd.Process.Pid)
	maxMemory := uint64(0)
	ch := make(chan error)
	go func() {
		ch <- cmd.Wait()
	}()
	running := true
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for running {
		select {
		case err := <-ch:
			if err != nil {
				return fmt.Errorf("Runtime Error #%v ... %v", sampleID, err.Error())
			}
			running = false
		case <-ticker.C:
			p, err := process.NewProcess(pid)
			if err == nil {
				m, err := p.MemoryInfo()
				if err == nil && m.RSS > maxMemory {
					maxMemory = m.RSS
				}
			}
		}
	}

	b, err := ioutil.ReadFile(ansPath)
	if err != nil {
		b = []byte{}
	}
	ans := plain(b)
	out := plain(o.Bytes())

	state := ""
	diff := ""
	if out == ans {
		state = color.New(color.FgGreen).Sprintf("Passed #%v", sampleID)
	} else {
		input, err := ioutil.ReadFile(inPath)
		if err != nil {
			return err
		}
		state = color.New(color.FgRed).Sprintf("Failed #%v", sampleID)
		dmp := diffmatchpatch.New()
		d := dmp.DiffMain(out, ans, true)
		diff += color.New(color.FgCyan).Sprintf("-----Input-----\n")
		diff += string(input) + "\n"
		diff += color.New(color.FgCyan).Sprintf("-----Output-----\n")
		diff += dmp.DiffText1(d) + "\n"
		diff += color.New(color.FgCyan).Sprintf("-----Answer-----\n")
		diff += dmp.DiffText2(d) + "\n"
		diff += color.New(color.FgCyan).Sprintf("-----Diff-----\n")
		diff += dmp.DiffPrettyText(d) + "\n"
	}

	parseMemory := func(memory uint64) string {
		if memory > 1024*1024 {
			return fmt.Sprintf("%.3fMB", float64(memory)/1024.0/1024.0)
		} else if memory > 1024 {
			return fmt.Sprintf("%.3fKB", float64(memory)/1024.0)
		}
		return fmt.Sprintf("%vB", memory)
	}

	ansi.Printf("%v ... %.3fs %v\n%v", state, cmd.ProcessState.UserTime().Seconds(), parseMemory(maxMemory), diff)
	if out != ans {
		return errors.New("Wrong answer")
	}
	return nil
}

type limitedOutput struct {
	writer    io.Writer
	remaining int
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New("Output limit exceeded")
	}
	n, e := w.writer.Write(p)
	w.remaining -= n
	return n, e
}

// Test command
func Test() (err error) {
	cfg := config.Instance
	if len(cfg.Template) == 0 {
		return errors.New("You have to add at least one code template by `cf config`")
	}
	samples := getSampleID()
	if len(samples) == 0 {
		return errors.New("Cannot find any sample file")
	}
	filename, index, err := getOneCode(Args.File, cfg.Template)
	if err != nil {
		return
	}
	template := cfg.Template[index]
	path, full := filepath.Split(filename)
	ext := filepath.Ext(filename)
	file := full[:len(full)-len(ext)]
	rand := util.RandString(8)

	filter := func(cmd string) string {
		cmd = strings.ReplaceAll(cmd, "$%rand%$", rand)
		cmd = strings.ReplaceAll(cmd, "$%path%$", path)
		cmd = strings.ReplaceAll(cmd, "$%full%$", full)
		cmd = strings.ReplaceAll(cmd, "$%file%$", file)
		return cmd
	}

	run := func(script string) error {
		if s := filter(script); len(s) > 0 {
			fmt.Println(s)
			cmds := splitCmd(s)
			if len(cmds) == 0 {
				return errors.New("Empty script command")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
			cmd.WaitDelay = time.Second
			cmd.Stdout = &limitedOutput{writer: os.Stdout, remaining: 1_000_000}
			cmd.Stderr = &limitedOutput{writer: os.Stderr, remaining: 1_000_000}
			return cmd.Run()
		}
		return nil
	}

	if err = run(template.BeforeScript); err != nil {
		return
	}
	var failures error
	if s := filter(template.Script); len(s) > 0 {
		for _, i := range samples {
			err := judge(i, s)
			if err != nil {
				color.Red(err.Error())
				failures = errors.Join(failures, err)
			}
		}
	} else {
		return errors.New("Invalid script command. Please check config file")
	}
	return errors.Join(failures, run(template.AfterScript))
}
