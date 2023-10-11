package pluginx

import (
	"log/slog"
	"os"
)

// logger is the default logger for pluginx.
var logger = slog.Default()
var stdout = os.Stdout
var stderr = os.Stderr

func SetLogger(l *slog.Logger) {
	logger = l
}

func SetStdout(s *os.File) {
	stdout = s
}

func SetStderr(s *os.File) {
	stderr = s
}
