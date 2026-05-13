package logger

import (
	"os"
	"sync"
	"time"

	lumberjack "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
)

// dailyRotateWriter wraps lumberjack and triggers rotate when local day changes.
type dailyRotateWriter struct {
	mu         sync.Mutex
	inner      *lumberjack.Logger
	rotateDaily bool
	lastDay    int
}

func newDailyRotateWriter(inner *lumberjack.Logger, rotateDaily bool) *dailyRotateWriter {
	return &dailyRotateWriter{
		inner:       inner,
		rotateDaily: rotateDaily,
		lastDay:     currentLocalDay(),
	}
}

func (w *dailyRotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rotateDaily {
		day := currentLocalDay()
		if day != w.lastDay && fileExists(w.inner.Filename) {
			_ = w.inner.Rotate()
			w.lastDay = day
		}
	}
	return w.inner.Write(p)
}

func (w *dailyRotateWriter) Sync() error {
	return nil
}

func currentLocalDay() int {
	now := time.Now()
	year, month, day := now.Date()
	return year*10000 + int(month)*100 + day
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

