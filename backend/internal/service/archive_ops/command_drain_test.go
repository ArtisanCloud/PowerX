package archive_ops

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type commandChildProof struct {
	PID  int `json:"pid"`
	PGID int `json:"pgid"`
	Port int `json:"port"`
}

func writeChildProof(root, name string, p commandChildProof) {
	b, _ := json.Marshal(p)
	_ = os.WriteFile(filepath.Join(root, name), b, 0600)
}

// 此帮助进程只由自有测试显式启动；参数中的临时根用于核实清理归属。
func TestArchiveCommandOwnedChild(t *testing.T) {
	mode := os.Getenv("POWERX_COMMAND_CHILD")
	if mode == "" {
		t.Skip("owned helper only")
	}
	root := os.Getenv("POWERX_COMMAND_CHILD_ROOT")
	if !strings.Contains(root, "command-drain") {
		t.Fatal("missing owned root")
	}
	switch mode {
	case "tree":
		signal.Ignore(syscall.SIGTERM)
		cmd := exec.Command(os.Args[0], "-test.run=^TestArchiveCommandOwnedChild$", "--", root)
		cmd.Env = append(os.Environ(), "POWERX_COMMAND_CHILD=descendant")
		require.NoError(t, cmd.Start())
		group, e := syscall.Getpgid(os.Getpid())
		require.NoError(t, e)
		writeChildProof(root, "leader.json", commandChildProof{PID: os.Getpid(), PGID: group})
		for {
			time.Sleep(time.Hour)
		}
	case "descendant":
		signal.Ignore(syscall.SIGTERM)
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, e)
		defer listener.Close()
		file, e := os.Create(filepath.Join(root, "owned-open-file"))
		require.NoError(t, e)
		defer file.Close()
		group, e := syscall.Getpgid(os.Getpid())
		require.NoError(t, e)
		writeChildProof(root, "descendant.json", commandChildProof{PID: os.Getpid(), PGID: group, Port: listener.Addr().(*net.TCPAddr).Port})
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	case "success":
		return
	case "crash-parent":
		cfg, e := LoadConfig(filepath.Join(root, "config.yaml"))
		require.NoError(t, e)
		s := NewService(context.Background(), cfg, Options{})
		j := jobRecord{Job: Job{ID: os.Getenv("POWERX_COMMAND_JOB"), PolicyID: os.Getenv("POWERX_COMMAND_POLICY"), SourceID: "media", Kind: "media", Status: "running", Phase: "capturing", CreatedAt: time.Now().UTC()}, RequestKey: strings.Repeat("f", 64)}
		finish, e := s.beginExecution(&j)
		require.NoError(t, e)
		defer finish()
		require.NoError(t, s.saveJob(&j))
		cmd := ownedCommand(root, "tree")
		_ = s.runArchiveCommand(context.Background(), cmd, j.ID, "pg_dump")
	}
}
func ownedCommand(root, mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestArchiveCommandOwnedChild$", "--", root)
	cmd.Env = append(os.Environ(), "POWERX_COMMAND_CHILD="+mode, "POWERX_COMMAND_CHILD_ROOT="+root)
	return cmd
}
func readChild(t *testing.T, root, name string) commandChildProof {
	t.Helper()
	var p commandChildProof
	require.Eventually(t, func() bool {
		b, e := os.ReadFile(filepath.Join(root, name))
		return e == nil && json.Unmarshal(b, &p) == nil && p.PID > 0
	}, 10*time.Second, 10*time.Millisecond)
	return p
}
func killOwnedTestCommandGroup(root string, p commandChildProof) {
	raw, e := exec.Command("ps", "-p", strconv.Itoa(p.PID), "-o", "command=").Output()
	if e != nil || !strings.Contains(string(raw), root) || !strings.Contains(string(raw), "TestArchiveCommandOwnedChild") {
		return
	}
	if group, e := syscall.Getpgid(p.PID); e == nil && group == p.PGID {
		_ = syscall.Kill(-group, syscall.SIGKILL)
	}
}
func assertChildGone(t *testing.T, p commandChildProof) {
	t.Helper()
	require.Eventually(t, func() bool { return errors.Is(syscall.Kill(p.PID, 0), syscall.ESRCH) }, 3*time.Second, 10*time.Millisecond)
	if p.Port > 0 {
		c, e := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p.Port), 100*time.Millisecond)
		if e == nil {
			c.Close()
			t.Fatal("owned child listener remains")
		}
	}
}
func TestCommandDrainCancellationAndCloseWaitForWholeLocalGroup(t *testing.T) {
	for _, mode := range []string{"cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			s, _, p := fixture(t)
			root, e := os.MkdirTemp("", "powerx-command-drain-")
			require.NoError(t, e)
			defer os.RemoveAll(root)
			j, _ := interruptedFixture(t, s, p, "running", "not_requested")
			run, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := ownedCommand(root, "tree")
			done := make(chan error, 1)
			go func() { done <- s.runArchiveCommand(run, cmd, j.ID, "ssh") }()
			leader := readChild(t, root, "leader.json")
			child := readChild(t, root, "descendant.json")
			t.Logf("owned command leader=%d child=%d pgid=%d port=%d cwd=%s command=%q", leader.PID, child.PID, leader.PGID, child.Port, root, cmd.Args)
			t.Cleanup(func() {
				killOwnedTestCommandGroup(root, leader)
			})
			require.Equal(t, leader.PGID, child.PGID)
			require.Equal(t, 1, s.commandOverview().Active)
			if mode == "close" {
				s.Close()
			} else {
				cancel()
			}
			require.Error(t, <-done)
			r, e := s.readCommandRecord(j.ID)
			require.NoError(t, e)
			require.Equal(t, "drained", r.State)
			assertChildGone(t, leader)
			assertChildGone(t, child)
			require.NoError(t, s.requireCommandsDrained(false))
		})
	}
}
func TestCommandDrainUnknownCorruptMissingAndLegacyNeverGrantDisposition(t *testing.T) {
	for _, scene := range []string{"pending", "corrupt", "missing", "legacy", "foreign_host"} {
		t.Run(scene, func(t *testing.T) {
			s, p, j, _ := dispositionFixture(t)
			ctx := credentialRootContext()
			objects := restartObjects(t, s.cfg.Directory)
			r, e := s.readCommandRecord(j.ID)
			require.NoError(t, e)
			switch scene {
			case "pending":
				r.State = "prepared"
				r.Kind = "pg_dump"
				r.CommandID = uuid.NewString()
				require.NoError(t, dispositionWrite(s.path("command-executions", j.ID), r))
			case "corrupt":
				require.NoError(t, os.WriteFile(s.path("command-executions", j.ID), []byte("broken"), 0600))
			case "missing":
				require.NoError(t, os.Remove(s.path("command-executions", j.ID)))
			case "legacy":
				j.CommandProtocol = 0
				require.NoError(t, s.saveJob(&j))
			case "foreign_host":
				r.Host = "another-host"
				require.NoError(t, dispositionWrite(s.path("command-executions", j.ID), r))
			}
			_, e = s.ReviewInterruptedCapture(ctx, j.ID, nil)
			require.Error(t, e)
			require.Equal(t, objects, restartObjects(t, s.cfg.Directory))
			current, e := s.readJob(j.ID)
			require.NoError(t, e)
			require.True(t, current.InterruptionPending)
			_, e = s.Retry(ctx, j.ID)
			require.Error(t, e)
			{
				_, e = s.Start(ctx, p.ID, uuid.NewString(), "owned", "")
				require.Equal(t, 409, dto.StatusCode(e))
			}
		})
	}
}
func TestCommandDrainSuccessAndStartFailureLeaveDurableDrainedRecord(t *testing.T) {
	s, _, p := fixture(t)
	j, _ := interruptedFixture(t, s, p, "running", "not_requested")
	root, e := os.MkdirTemp("", "powerx-command-drain-")
	require.NoError(t, e)
	defer os.RemoveAll(root)
	require.NoError(t, s.runArchiveCommand(context.Background(), ownedCommand(root, "success"), j.ID, "rsync"))
	require.Error(t, s.runArchiveCommand(context.Background(), exec.Command(filepath.Join(root, "missing")), j.ID, "rsync"))
	r, e := s.readCommandRecord(j.ID)
	require.NoError(t, e)
	require.Equal(t, "drained", r.State)
}
func TestCommandDrainSIGKILLParentRetainsUnknownOriginalAndNeverSignalsOldPID(t *testing.T) {
	s, _, p := fixture(t)
	root, e := os.MkdirTemp("", "powerx-command-drain-")
	require.NoError(t, e)
	defer os.RemoveAll(root)
	job := uuid.NewString()
	raw, e := marshalConfig(s.cfg)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), raw, 0600))
	cmd := ownedCommand(root, "crash-parent")
	cmd.Env = append(cmd.Env, "POWERX_COMMAND_JOB="+job, "POWERX_COMMAND_POLICY="+p.ID)
	require.NoError(t, cmd.Start())
	leader := readChild(t, root, "leader.json")
	child := readChild(t, root, "descendant.json")
	t.Logf("owned parent=%d leader=%d child=%d pgid=%d port=%d cwd=%s command=%q", cmd.Process.Pid, leader.PID, child.PID, leader.PGID, child.Port, root, cmd.Args)
	defer func() {
		// 清理仅针对记录了本测试根的自有存活命令组，不把台账中的裸 PID 当作通用杀进程入口。
		killOwnedTestCommandGroup(root, leader)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		assertChildGone(t, leader)
		assertChildGone(t, child)
	}()
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	peer := NewService(context.Background(), s.cfg, Options{})
	defer peer.Close()
	require.NoError(t, peer.initErr)
	old, e := peer.Job(archiveRoot(), job)
	require.NoError(t, e)
	require.True(t, old.InterruptionPending)
	require.Equal(t, "unconfirmed", old.CommandDrainState)
	require.NoError(t, syscall.Kill(leader.PID, 0), "restart must not signal a recorded old PID")
	before, e := os.ReadFile(peer.path("command-executions", job))
	require.NoError(t, e)
	_, e = peer.ReviewInterruptedCapture(credentialRootContext(), job, nil)
	require.Equal(t, 409, dto.StatusCode(e))
	_, e = peer.Start(archiveRoot(), p.ID, uuid.NewString(), "owned", "")
	require.Equal(t, 409, dto.StatusCode(e))
	_, e = peer.cleanupApproved(archiveRoot(), p.ID)
	require.Equal(t, 409, dto.StatusCode(e))
	after, e := os.ReadFile(peer.path("command-executions", job))
	require.NoError(t, e)
	require.Equal(t, before, after)
}

func TestCommandDrainHistoricalClosureCannotRestoreCurrentProof(t *testing.T) {
	s, _, j, r := unknownDispositionFixture(t)
	ctx := credentialRootContext()
	review, e := s.ReviewInterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	receipt, e := s.CloseInterruptionRequest(ctx, r.RequestID, closureInput(review))
	require.NoError(t, e)
	require.True(t, receipt.CurrentProtectionMatches)
	before, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	require.NoError(t, os.Remove(s.path("command-executions", j.ID)))
	historical, e := s.InterruptionRequest(ctx, r.RequestID)
	require.NoError(t, e)
	require.Equal(t, "closed_unknown", historical.State)
	require.False(t, historical.CurrentProtectionMatches)
	after, e := os.ReadFile(s.path("interruption-requests", r.RequestID))
	require.NoError(t, e)
	require.Equal(t, before, after)
}
