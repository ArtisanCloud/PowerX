package logger

import (
	"sync"
	"testing"

	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
)

func TestInitGlobalLoggerOverridesDefaultLogger(t *testing.T) {
	prevInstance := globalInstance
	t.Cleanup(func() {
		globalMu.Lock()
		globalInstance = prevInstance
		globalMu.Unlock()
	})

	globalMu.Lock()
	globalInstance = nil
	globalMu.Unlock()

	if !GetGlobalLogger().config.Console {
		t.Fatalf("default global logger console = false, want true")
	}

	InitGlobalLogger(&logcfg.LogConfig{
		Level:   "debug",
		Console: false,
		File:    logcfg.FileConfig{Enable: false},
	})

	if GlobalConsoleEnabled() {
		t.Fatalf("GlobalConsoleEnabled() = true, want false after explicit init")
	}
}

func TestInitGlobalLoggerCanUpdateConsoleConfig(t *testing.T) {
	prevInstance := globalInstance
	t.Cleanup(func() {
		globalMu.Lock()
		globalInstance = prevInstance
		globalMu.Unlock()
	})

	InitGlobalLogger(&logcfg.LogConfig{
		Level:   "debug",
		Console: true,
		File:    logcfg.FileConfig{Enable: false},
	})
	if !GlobalConsoleEnabled() {
		t.Fatalf("GlobalConsoleEnabled() = false, want true")
	}

	InitGlobalLogger(&logcfg.LogConfig{
		Level:   "debug",
		Console: false,
		File:    logcfg.FileConfig{Enable: false},
	})
	if GlobalConsoleEnabled() {
		t.Fatalf("GlobalConsoleEnabled() = true, want false")
	}
}

func TestGetGlobalLoggerConcurrentDefaultInit(t *testing.T) {
	prevInstance := globalInstance
	t.Cleanup(func() {
		globalMu.Lock()
		globalInstance = prevInstance
		globalMu.Unlock()
	})

	globalMu.Lock()
	globalInstance = nil
	globalMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if GetGlobalLogger() == nil {
				t.Errorf("GetGlobalLogger() returned nil")
			}
		}()
	}
	wg.Wait()
}
