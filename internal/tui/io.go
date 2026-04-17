package tui

import (
	"io"
	"os"
)

// Output / input are variables so tests can redirect without touching globals.
func stderrWriter() io.Writer { return os.Stderr }
func stdinReader() io.Reader  { return os.Stdin }
