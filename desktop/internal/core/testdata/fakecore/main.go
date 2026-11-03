package main

// fakecore: the observable contract of a wisteria client core, without the
// protocol. The reconnect regression tests (service_reconnect_test.go)
// drive the shell's supervisor against this: bind the local SOCKS port =
// session alive; close it and keep running = the rekey/session death the
// production shell used to miss. Configured via env (see the test file).

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

func main() {
	mode := os.Getenv("FAKECORE_MODE")
	port := os.Getenv("FAKECORE_PORT")
	if pf := os.Getenv("FAKECORE_PIDFILE"); pf != "" {
		f, err := os.OpenFile(pf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
		}
	}
	addr := "127.0.0.1:" + port
	switch mode {
	case "exit_immediate":
		os.Exit(1)
	case "no_bind":
		select {} // alive, serving nothing: a dead server at connect time
	default:
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			os.Exit(2)
		}
		if mode == "close_after" {
			d := time.Second
			if v := os.Getenv("FAKECORE_CLOSE_AFTER"); v != "" {
				if s, err := strconv.ParseFloat(v, 64); err == nil {
					d = time.Duration(s * float64(time.Second))
				}
			}
			// Session death: the listener goes away, the process stays —
			// exactly the wisteria core's "redial in progress" shape.
			time.AfterFunc(d, func() { ln.Close() })
		}
		select {}
	}
}
