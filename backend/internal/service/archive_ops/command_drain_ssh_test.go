package archive_ops

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCommandDrainRealSSHAndRsyncChildDuringBlockedHandshake(t *testing.T) {
	for _, kind := range []string{"ssh", "rsync"} {
		t.Run(kind, func(t *testing.T) {
			credential, _ := sshFixture(t)
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, e)
			var mu sync.Mutex
			var connections []net.Conn
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					c, e := listener.Accept()
					if e != nil {
						return
					}
					mu.Lock()
					connections = append(connections, c)
					mu.Unlock()
				}
			}()
			t.Cleanup(func() {
				listener.Close()
				<-done
				mu.Lock()
				defer mu.Unlock()
				for _, c := range connections {
					c.Close()
				}
				c, e := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
				if e == nil {
					c.Close()
					t.Error("owned SSH blackhole port remains")
				}
			})
			s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "owned-commands", Credentials: map[string]Credential{"owned": credential}}, Options{})
			defer s.Close()
			target := Target{ID: uuid.NewString(), Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, User: "archive", CredentialRef: "owned", Directory: "/owned-test"}
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "owned.txt"), []byte("source unchanged"), 0600))
			job := uuid.NewString()
			run, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				if kind == "ssh" {
					_, e := s.remote(run, target, "test", uuid.NewString(), job, nil)
					result <- e
				} else {
					result <- s.rsync(run, target, root+"/", "/owned-test/objects/", false, job)
				}
			}()
			var record commandRecord
			var children []int
			require.Eventually(t, func() bool {
				record, e = s.readCommandRecord(job)
				if e != nil || record.State != "running" {
					return false
				}
				rows, e := exec.Command("ps", "-axo", "pid=,pgid=,comm=").Output()
				if e != nil {
					return false
				}
				children = nil
				for _, line := range strings.Split(string(rows), "\n") {
					fields := strings.Fields(line)
					if len(fields) < 3 {
						continue
					}
					group, _ := strconv.Atoi(fields[1])
					pid, _ := strconv.Atoi(fields[0])
					if group == record.PGID {
						children = append(children, pid)
					}
				}
				mu.Lock()
				accepted := len(connections) > 0
				mu.Unlock()
				return accepted && (kind == "ssh" && len(children) > 0 || kind == "rsync" && len(children) >= 2)
			}, 10*time.Second, 10*time.Millisecond)
			t.Logf("owned actual %s leader=%d pgid=%d children=%v receiver-port=%d cwd=%s command=%s <fixed args; credentials omitted>", kind, record.PID, record.PGID, children, target.Port, root, kind)
			cancel()
			require.Error(t, <-result)
			for _, pid := range children {
				assertChildGone(t, commandChildProof{PID: pid, PGID: record.PGID})
			}
			record, e = s.readCommandRecord(job)
			require.NoError(t, e)
			require.Equal(t, "drained", record.State)
			raw, e := os.ReadFile(filepath.Join(root, "owned.txt"))
			require.NoError(t, e)
			require.Equal(t, "source unchanged", string(raw))
		})
	}
}
