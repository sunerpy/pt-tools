// Command fake-pt-tools stands in for the pt-tools binary in deploy/fnos/native/test-fnos-fpk.sh.
//
// It accepts `web --host H --port P`, records its arguments and the environment variables that
// cmd/main sets into $FAKE_RECORD, listens on H:P and exits on SIGTERM. FAKE_IGNORE_TERM=1 makes it
// ignore SIGTERM (cmd/main must escalate to SIGKILL), FAKE_EXIT=1 makes it write "fake boom" to
// stderr and exit 1. It never runs longer than FAKE_MAX_SECONDS (default 120), so a failed test
// cannot leave it behind.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "web" {
		fmt.Fprintln(os.Stderr, "fake-pt-tools: only the web command is supported")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	host := fs.String("host", "", "")
	port := fs.Int("port", 0, "")
	_ = fs.Parse(os.Args[2:])

	if os.Getenv("FAKE_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "fake boom")
		os.Exit(1)
	}

	// SIGTERM 的处理在写记录之前装好：测试一看到记录就可能发信号
	term := make(chan os.Signal, 1)
	if os.Getenv("FAKE_IGNORE_TERM") == "1" {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(term, syscall.SIGTERM)
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-pt-tools:", err)
		os.Exit(1)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	if path := os.Getenv("FAKE_RECORD"); path != "" {
		wd, _ := os.Getwd()
		lines := []string{
			"args=" + strings.Join(os.Args[1:], " "),
			"HOME=" + os.Getenv("HOME"),
			"PT_TOOLS_LOG_CONSOLE=" + os.Getenv("PT_TOOLS_LOG_CONSOLE"),
			"PT_TOOLS_MANAGED_BY=" + os.Getenv("PT_TOOLS_MANAGED_BY"),
			"PWD=" + wd,
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "fake-pt-tools:", err)
			os.Exit(1)
		}
	}

	limit := 120 * time.Second
	if v, err := strconv.Atoi(os.Getenv("FAKE_MAX_SECONDS")); err == nil && v > 0 {
		limit = time.Duration(v) * time.Second
	}
	select {
	case <-term:
	case <-time.After(limit):
	}
	_ = ln.Close()
}
