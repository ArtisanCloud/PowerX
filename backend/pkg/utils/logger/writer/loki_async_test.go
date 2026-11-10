package writer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"github.com/stretchr/testify/require"
)

func TestLokiSlowProviderNeverBlocksBusinessAndMemoryIsBounded(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(204)
	}))
	defer server.Close()
	defer close(release)
	w := NewLokiWriter(&logcfg.LokiConfig{URL: server.URL, BatchSize: 1, QueueCapacity: 2, MaxQueueBytes: 128, MaxEntryBytes: 64, RequestTimeoutMS: 50, FlushTimeoutMS: 100})
	_, err := w.Write([]byte("first"))
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	begin := time.Now()
	for i := 0; i < 1000; i++ {
		_, err = w.Write([]byte(strings.Repeat("x", 64)))
		require.NoError(t, err)
	}
	require.Less(t, time.Since(begin), 200*time.Millisecond)
	s := w.Stats()
	require.Positive(t, s.Dropped)
	require.LessOrEqual(t, s.QueuedBytes, int64(128))
	require.LessOrEqual(t, s.QueuedEntries, int64(3))
	_ = w.Close()
	require.Zero(t, w.Stats().QueuedBytes)
	require.True(t, w.Stats().Closed)
}

func TestLokiBatchFlushCopiesEntriesAndPreservesLowCardinalityLabels(t *testing.T) {
	var mu sync.Mutex
	var values [][]string
	var labels map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Streams []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"streams"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		labels = body.Streams[0].Stream
		values = append(values, body.Streams[0].Values...)
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer server.Close()
	w := NewLokiWriter(&logcfg.LokiConfig{URL: server.URL, BatchSize: 2, Labels: map[string]string{"env": "prod"}})
	defer w.Close()
	p := []byte("original")
	_, err := w.Write(p)
	require.NoError(t, err)
	copy(p, []byte("changed!"))
	_, err = w.Write([]byte("second"))
	require.NoError(t, err)
	require.NoError(t, w.Sync())
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, values, 2)
	require.Equal(t, "original", values[0][1])
	require.Equal(t, "prod", labels["env"])
	s := w.Stats()
	require.EqualValues(t, 2, s.Delivered)
	require.Zero(t, s.QueuedBytes)
}

func TestLokiRetriesAreBoundedAndNeverExposeProviderBody(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
		_, _ = w.Write([]byte("private-provider-body"))
	}))
	defer server.Close()
	w := NewLokiWriter(&logcfg.LokiConfig{URL: server.URL, BatchSize: 1})
	defer w.Close()
	_, err := w.Write([]byte("business log"))
	require.NoError(t, err)
	require.NoError(t, w.Sync())
	require.EqualValues(t, 3, calls.Load())
	s := w.Stats()
	require.EqualValues(t, 1, s.Failed)
	require.EqualValues(t, 2, s.Retries)
	require.Equal(t, "http_500", s.LastError)
	require.NotContains(t, s.LastError, "private")
}

func TestLokiConcurrentCloseAndOversizeAreSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	w := NewLokiWriter(&logcfg.LokiConfig{URL: server.URL, BatchSize: 4, MaxEntryBytes: 8, QueueCapacity: 8})
	_, err := w.Write([]byte("too large for this queue"))
	require.NoError(t, err)
	require.EqualValues(t, 1, w.Stats().Dropped)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				_, _ = w.Write([]byte("line"))
			}
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = w.Close() }()
	wg.Wait()
	_ = w.Close()
	require.Zero(t, w.Stats().QueuedBytes)
	require.Zero(t, w.Stats().QueuedEntries)
}
