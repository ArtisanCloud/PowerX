package supervisor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPinnedExecutableRejectsInterpretersAndFreezesActualBytes(t *testing.T) {
	script := filepath.Join(t.TempDir(), "plugin.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0700))
	_, _, _, err := freezeExecutable(script)
	require.ErrorIs(t, err, ErrUnpinnedProcess)
	goBinary, err := exec.LookPath("go")
	require.NoError(t, err)
	_, _, _, err = freezeExecutable(goBinary)
	require.ErrorIs(t, err, ErrUnpinnedProcess)
	original, err := os.Executable()
	require.NoError(t, err)
	bytes, err := os.ReadFile(original)
	require.NoError(t, err)
	installed := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.WriteFile(installed, bytes, 0700))
	frozen, directory, digest, err := freezeExecutable(installed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	require.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256(bytes)), digest)
	require.NoError(t, os.WriteFile(installed, []byte("changed"), 0700))
	actual, err := os.ReadFile(frozen)
	require.NoError(t, err)
	require.Equal(t, bytes, actual)
}

func TestPinnedHelper(t *testing.T) {
	if os.Getenv("POWERX_PIN_TEST_HELPER") != "1" {
		return
	}
	server := &http.Server{Addr: os.Getenv("POWERX_HTTP_ADDR"), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }), ReadHeaderTimeout: time.Second}
	if err := server.ListenAndServe(); err != nil {
		os.Exit(2)
	}
}

func TestPinnedStopWaitsForInvocationAndReleasesResources(t *testing.T) {
	binary, err := os.Executable()
	require.NoError(t, err)
	sup := New()
	id := "test.pinned.helper"
	port, err := sup.Start(context.Background(), id, binary, []string{"-test.run=^TestPinnedHelper$"}, map[string]string{"POWERX_PIN_TEST_HELPER": "1", "POWERX_INTERNAL_TOKEN": "fixture"}, Options{PinExecutable: true, HealthPath: "/healthz", HealthInterval: 500 * time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sup.Stop(id))
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if connection != nil {
			_ = connection.Close()
		}
		require.Error(t, err)
	})
	require.Eventually(t, func() bool { _, err := sup.Pin(id); return err == nil }, 15*time.Second, 20*time.Millisecond)
	sup.mu.RLock()
	imageDir := sup.proc[id].imageDir
	pid := sup.proc[id].info.PID
	sup.mu.RUnlock()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Logf("agent-owned PID=%d command=%s -test.run=^TestPinnedHelper$ cwd=%s port=%d cleanup=Supervisor.Stop", pid, binary, cwd, port)
	pin, err := sup.Pin(id)
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	invoked := make(chan error, 1)
	go func() {
		invoked <- sup.WithPinnedProcess(context.Background(), id, pin, func(actual int, token string) error {
			if actual != port || token != "fixture" {
				return ErrUnpinnedProcess
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- sup.Stop(id) }()
	select {
	case err := <-stopped:
		t.Fatalf("stop interleaved with invocation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-invoked)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(8 * time.Second):
		t.Fatal("stop deadlocked")
	}
	_, err = os.Stat(imageDir)
	require.True(t, os.IsNotExist(err))
	require.ErrorIs(t, sup.WithPinnedProcess(context.Background(), id, pin, func(int, string) error { return nil }), ErrUnpinnedProcess)
	_, err = sup.Start(context.Background(), id, binary, nil, nil, Options{PinExecutable: true, AutoRestart: true})
	require.ErrorIs(t, err, ErrUnpinnedProcess)
}
