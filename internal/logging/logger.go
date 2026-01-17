package logging

import (
	"fmt"
	"io"
	"time"
)

// Logger provides optional verbose logging and lightweight timing helpers.
type Logger struct {
	Writer  io.Writer
	Verbose bool
}

func New(writer io.Writer, verbose bool) Logger {
	return Logger{Writer: writer, Verbose: verbose}
}

func (l Logger) Infof(format string, args ...any) {
	if l.Writer == nil {
		return
	}
	fmt.Fprintf(l.Writer, format+"\n", args...)
}

func (l Logger) Verbosef(format string, args ...any) {
	if !l.Verbose {
		return
	}
	l.Infof("Verbose: "+format, args...)
}

// Measure returns a stop function that logs the elapsed time when called.
func (l Logger) Measure(label string) func() {
	if !l.Verbose {
		return func() {}
	}
	start := time.Now()
	return func() {
		elapsed := time.Since(start).Round(time.Millisecond)
		l.Verbosef("%s took %s", label, elapsed)
	}
}
