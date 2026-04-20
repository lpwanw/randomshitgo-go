// Command listen is a test helper that binds a TCP listener on a random port,
// prints the chosen port to stdout, then sleeps until stdin is closed.
// Used by internal/netinfo port tests to provide a real PID+port pair.
package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	defer ln.Close()

	// Print the port so the test can read it.
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Println(port)
	os.Stdout.Sync() //nolint:errcheck

	// Block until stdin is closed (test harness closes the write-end of stdin
	// to signal shutdown, or the test kills the process directly).
	io.ReadAll(os.Stdin) //nolint:errcheck
}
