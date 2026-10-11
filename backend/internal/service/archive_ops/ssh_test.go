package archive_ops

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 独立回环 SSH 接收器，使用临时密钥，真实运行 Python3 和 rsync。
func sshFixture(t *testing.T) (Credential, int) {
	t.Helper()
	for _, tool := range []string{"ssh", "ssh-keygen", "rsync", "python3"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skip(tool + " unavailable")
		}
	}
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).Run())
	pubRaw, e := os.ReadFile(key + ".pub")
	require.NoError(t, e)
	pub, _, _, _, e := ssh.ParseAuthorizedKey(pubRaw)
	require.NoError(t, e)
	_, hostPrivate, e := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, e)
	signer, e := ssh.NewSignerFromKey(hostPrivate)
	require.NoError(t, e)
	config := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if c.User() == "archive" && string(key.Marshal()) == string(pub.Marshal()) {
			return nil, nil
		}
		return nil, os.ErrPermission
	}}
	config.AddHostKey(signer)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	port := listener.Addr().(*net.TCPAddr).Port
	known := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(known, []byte("[127.0.0.1]:"+strconv.Itoa(port)+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))+"\n"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	var mu sync.Mutex
	connections := []net.Conn{}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			connections = append(connections, conn)
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				server, chans, reqs, e := ssh.NewServerConn(conn, config)
				if e != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, e := ch.Accept()
					if e != nil {
						continue
					}
					workers.Add(1)
					go func() {
						defer workers.Done()
						defer channel.Close()
						for request := range requests {
							if request.Type != "exec" {
								request.Reply(false, nil)
								continue
							}
							var payload struct{ Command string }
							if ssh.Unmarshal(request.Payload, &payload) != nil {
								request.Reply(false, nil)
								return
							}
							request.Reply(true, nil)
							cmd := exec.CommandContext(ctx, "/bin/sh", "-c", payload.Command)
							stdin, e := cmd.StdinPipe()
							if e != nil {
								return
							}
							go func() { io.Copy(stdin, channel); stdin.Close() }()
							cmd.Stdout = channel
							cmd.Stderr = channel.Stderr()
							e = cmd.Run()
							status := uint32(0)
							if e != nil {
								status = 1
							}
							buffer := make([]byte, 4)
							binary.BigEndian.PutUint32(buffer, status)
							channel.SendRequest("exit-status", false, buffer)
							return
						}
					}()
				}
			}()
		}
	}()
	t.Logf("owned SSH fixture pid=%d listener=127.0.0.1:%d cwd=%s; temporary signed-key server", os.Getpid(), port, t.TempDir())
	t.Cleanup(func() {
		cancel()
		listener.Close()
		mu.Lock()
		for _, conn := range connections {
			conn.Close()
		}
		mu.Unlock()
		workers.Wait()
		c, e := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
		if e == nil {
			c.Close()
			t.Error("fixture port still listening")
		}
	})
	return Credential{KeyFile: key, KnownHostsFile: known}, port
}
func TestSSHLogArchiveRetentionAndRemoteRecovery(t *testing.T) {
	cred, port := sshFixture(t)
	source := t.TempDir()
	current := filepath.Join(t.TempDir(), "current_logfiles")
	require.NoError(t, os.WriteFile(current, []byte("stderr logs/active.log\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "active.log"), []byte("still writing"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "closed.log"), []byte("DELETE audit record\n"), 0600))
	remoteRoot := filepath.Join(t.TempDir(), "remote")
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "server-test", Sources: []Source{{ID: "pg", Kind: "postgresql_logs", Directory: source, CurrentLogfiles: current}}, Credentials: map[string]Credential{"test": cred}}, Options{})
	t.Cleanup(s.Close)
	target, e := s.SaveTarget(archiveRoot(), Target{Name: "remote", Host: "127.0.0.1", Port: port, User: "archive", Directory: remoteRoot, CredentialRef: "test"})
	require.NoError(t, e)
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "SQL logs", SourceID: "pg", IntervalMinutes: 1, RetentionDays: 1, RetentionCount: 1, RemoteRetentionDays: 90, RemoteRetentionCount: 90, TargetID: target.ID})
	require.NoError(t, e)
	first := captureTest(t, s, p)
	require.Equal(t, "success", first.Status, first.Error)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), first.ID); return e == nil && j.RemoteState != "pending" }, 30*time.Second, 50*time.Millisecond)
	first, e = s.Job(archiveRoot(), first.ID)
	require.NoError(t, e)
	require.Equal(t, "success", first.RemoteState, first.RemoteError)
	require.Equal(t, 1, first.FileCount)
	require.Equal(t, "postgresql_logs/closed.log", first.Files[0].Path)
	require.NoError(t, os.Remove(filepath.Join(source, "closed.log")))
	require.NoError(t, os.WriteFile(filepath.Join(source, "next.log"), []byte("new log"), 0600))
	second := captureTest(t, s, p)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), second.ID); return e == nil && j.RemoteState == "success" }, 30*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool { j, e := s.Job(archiveRoot(), first.ID); return e == nil && j.Status == "pruned" }, 10*time.Second, 50*time.Millisecond)
	_, e = os.Stat(filepath.Join(remoteRoot, s.cfg.Instance, p.ID, "manifests", first.ID+".json"))
	require.NoError(t, e)
	var data strings.Builder
	require.NoError(t, s.Download(archiveRoot(), first.ID, &data))
	require.Greater(t, len(data.String()), 0)
	_, e = s.Protect(archiveRoot(), second.ID, true)
	require.NoError(t, e)
	// 远端数量清理保护项，且不受本地保留影响。
	p.RemoteRetentionCount = 1
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(source, "third.log"), []byte("third log"), 0600))
	third := captureTest(t, s, p)
	require.Equal(t, "success", third.RemoteState, third.RemoteError)
	_, e = os.Stat(filepath.Join(remoteRoot, s.cfg.Instance, p.ID, "manifests", first.ID+".json"))
	require.True(t, os.IsNotExist(e))
	_, e = os.Stat(filepath.Join(remoteRoot, s.cfg.Instance, p.ID, "manifests", second.ID+".json"))
	require.NoError(t, e)
	// 错误主机信任必须失败，不能自动接受陌生服务器。
	bad := target
	bad.CredentialRef = "untrusted"
	s.cfg.Credentials["untrusted"] = Credential{KeyFile: cred.KeyFile, KnownHostsFile: filepath.Join(t.TempDir(), "empty")}
	require.NoError(t, os.WriteFile(s.cfg.Credentials["untrusted"].KnownHostsFile, nil, 0600))
	bad.ID = ""
	bad, e = s.SaveTarget(archiveRoot(), bad)
	require.NoError(t, e)
	require.Error(t, s.TestTarget(archiveRoot(), bad.ID))
	// 接收器只在自身 namespace 清理，原始日志和其他目录不触碰。
	unrelated := filepath.Join(remoteRoot, "keep.txt")
	require.NoError(t, os.WriteFile(unrelated, []byte("keep"), 0600))
	_, e = s.remote(archiveRoot(), target, "commit", p.ID, uuid.NewString(), map[string]any{"days": 1, "count": 1})
	require.Error(t, e)
	b, e := os.ReadFile(unrelated)
	require.NoError(t, e)
	require.Equal(t, "keep", string(b))
}
func TestSSHFailedTransferRetriesOriginalSnapshot(t *testing.T) {
	cred, port := sshFixture(t)
	known, e := os.ReadFile(cred.KnownHostsFile)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(cred.KnownHostsFile, nil, 0600))
	s, dir, p := fixture(t)
	s.cfg.Credentials = map[string]Credential{"test": cred}
	target, e := s.SaveTarget(archiveRoot(), Target{Name: "retry", Host: "127.0.0.1", Port: port, User: "archive", Directory: filepath.Join(t.TempDir(), "remote"), CredentialRef: "test"})
	require.NoError(t, e)
	p.TargetID = target.ID
	p, e = s.SavePolicy(archiveRoot(), p)
	require.NoError(t, e)
	j := captureTest(t, s, p)
	require.Equal(t, "success", j.Status)
	require.Equal(t, "failed", j.RemoteState)
	require.Equal(t, 1, j.Attempts)
	original := j.Files[0].SHA256
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("new content after failed transfer"), 0600))
	require.NoError(t, os.WriteFile(cred.KnownHostsFile, known, 0600))
	_, e = s.Retry(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		current, e := s.Job(archiveRoot(), j.ID)
		return e == nil && current.RemoteState == "success"
	}, 20*time.Second, 50*time.Millisecond)
	current, e := s.Job(archiveRoot(), j.ID)
	require.NoError(t, e)
	require.Equal(t, 2, current.Attempts)
	require.Equal(t, original, current.Files[0].SHA256)
	result, e := s.remote(archiveRoot(), target, "read", p.ID, j.ID, nil)
	require.NoError(t, e)
	require.NotNil(t, result["manifest"])
}
