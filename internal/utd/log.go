package utd

import (
	"fmt"
	"io"
)

// Level controls how talkative the logger is.
type Level int

const (
	// LevelQuiet only reports errors.
	LevelQuiet Level = iota
	// LevelNormal reports the important progress steps.
	LevelNormal
	// LevelVerbose reports every step, useful when debugging.
	LevelVerbose
)

const (
	colReset  = "\033[0m"
	colRed    = "\033[31m"
	colYellow = "\033[33m"
	colCyan   = "\033[36m"
	colDim    = "\033[2m"
)

// Logger writes human readable progress information to a single writer
// (usually stderr so that the wrapped command keeps full ownership of stdout).
type Logger struct {
	level Level
	w     io.Writer
	color bool
}

// NewLogger creates a logger. When color is true, messages are colorized.
func NewLogger(level Level, w io.Writer, color bool) *Logger {
	return &Logger{level: level, w: w, color: color}
}

// Level returns the configured verbosity.
func (l *Logger) Level() Level { return l.level }

func (l *Logger) paint(code, s string) string {
	if !l.color {
		return s
	}
	return code + s + colReset
}

// Errorf always prints, regardless of the verbosity level.
func (l *Logger) Errorf(format string, a ...any) {
	fmt.Fprintln(l.w, l.paint(colRed, "utd: error: ")+fmt.Sprintf(format, a...))
}

// Warnf prints unless the logger is quiet.
func (l *Logger) Warnf(format string, a ...any) {
	if l.level < LevelNormal {
		return
	}
	fmt.Fprintln(l.w, l.paint(colYellow, "utd: warning: ")+fmt.Sprintf(format, a...))
}

// Infof prints at the normal level and above.
func (l *Logger) Infof(format string, a ...any) {
	if l.level < LevelNormal {
		return
	}
	fmt.Fprintln(l.w, l.paint(colCyan, "utd: ")+fmt.Sprintf(format, a...))
}

// Verbosef prints only in verbose mode.
func (l *Logger) Verbosef(format string, a ...any) {
	if l.level < LevelVerbose {
		return
	}
	fmt.Fprintln(l.w, l.paint(colDim, "utd: ")+fmt.Sprintf(format, a...))
}
