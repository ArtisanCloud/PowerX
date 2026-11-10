package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mediaOutputReaderFunc func([]byte) (int, error)

func (f mediaOutputReaderFunc) Read(p []byte) (int, error) { return f(p) }

func mediaOutputBudgetFixture(t *testing.T) (*budget.Coordinator, string) {
	t.Helper()
	root := t.TempDir()
	downloads := filepath.Join(root, "downloads")
	require.NoError(t, os.Mkdir(downloads, 0700))
	c, err := budget.New(budget.Config{
		Directory: filepath.Join(root, "budget"), FilesystemPath: downloads,
		HostID: "media-test-host", FilesystemID: "media-test-filesystem", InstanceID: "media-test-instance",
		MinFreeBytes: 1 << 20, CriticalFreeBytes: 1, MaxConcurrent: 1,
	})
	require.NoError(t, err)
	t.Setenv("TMPDIR", downloads)
	budget.SetDefault(c)
	t.Cleanup(func() { budget.SetDefault(nil) })
	return c, downloads
}

func requireMediaOutputReleased(t *testing.T, c *budget.Coordinator, downloads string) {
	t.Helper()
	snapshot, err := c.Status(context.Background())
	require.NoError(t, err)
	require.Empty(t, snapshot.Active)
	files, err := filepath.Glob(filepath.Join(downloads, "powerx-ai-media-*"))
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestMediaOutputAdmissionRejectsBeforeCreatingOrReading(t *testing.T) {
	c, downloads := mediaOutputBudgetFixture(t)
	policy, err := c.Policy(context.Background())
	require.NoError(t, err)
	policy.MinFreeBytes = 1 << 50 // 只修改自有账本，不占用真实磁盘。
	_, err = c.SavePolicy(context.Background(), policy.Revision, policy)
	require.NoError(t, err)
	reads := 0
	body := mediaOutputReaderFunc(func(p []byte) (int, error) {
		reads++
		return copy(p, "unprotected output"), io.EOF
	})
	_, err = (&MediaRuntime{}).storeOutput(context.Background(), "tenant", "task", 0, "image", body)
	require.ErrorIs(t, err, budget.ErrInsufficient)
	require.Equal(t, "AI_MEDIA_STORAGE_CAPACITY_EXHAUSTED", mediaErrorCode(err))
	require.Zero(t, reads)
	requireMediaOutputReleased(t, c, downloads)
}

func TestMediaOutputAdmissionHonorsConcurrentReservation(t *testing.T) {
	c, downloads := mediaOutputBudgetFixture(t)
	lease, err := c.Reserve(context.Background(), "another-upload", 1, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lease.Release(context.Background())) })
	reads := 0
	_, err = (&MediaRuntime{}).storeOutput(context.Background(), "tenant", "task", 0, "image", mediaOutputReaderFunc(func(p []byte) (int, error) {
		reads++
		return copy(p, "unprotected output"), io.EOF
	}))
	require.ErrorIs(t, err, budget.ErrBusy)
	require.Equal(t, "AI_MEDIA_STORAGE_UNAVAILABLE", mediaErrorCode(err))
	require.Zero(t, reads)
	require.NoError(t, lease.Release(context.Background()))
	requireMediaOutputReleased(t, c, downloads)
}

func TestMediaOutputAdmissionReleasesWhenTemporaryCreationFails(t *testing.T) {
	c, downloads := mediaOutputBudgetFixture(t)
	t.Setenv("TMPDIR", filepath.Join(downloads, "missing"))
	_, err := (&MediaRuntime{}).storeOutput(context.Background(), "tenant", "task", 0, "image", bytes.NewReader(nil))
	require.ErrorIs(t, err, os.ErrNotExist)
	requireMediaOutputReleased(t, c, downloads)
}

func TestMediaOutputAdmissionReleasesOnReadFailureAndCancellation(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		name := "read_failure"
		if cancelRead {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			c, downloads := mediaOutputBudgetFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			readError := errors.New("test download failed")
			body := mediaOutputReaderFunc(func(p []byte) (int, error) {
				snapshot, err := c.Status(context.Background())
				require.NoError(t, err)
				require.Len(t, snapshot.Active, 1, "下载读取之前必须已有租约")
				if cancelRead {
					cancel()
					return copy(p, "must not write after cancellation"), io.EOF
				}
				return copy(p, "partial download"), readError
			})
			_, err := (&MediaRuntime{}).storeOutput(ctx, "tenant", "task", 0, "image", body)
			if cancelRead {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, readError)
			}
			requireMediaOutputReleased(t, c, downloads)
		})
	}
}

func TestMediaOutputAdmissionStopsBeforeUnreservedGrowth(t *testing.T) {
	c, downloads := mediaOutputBudgetFixture(t)
	var written, peak int64
	var temporary string
	body := mediaOutputReaderFunc(func(p []byte) (int, error) {
		if peak == 0 {
			snapshot, err := c.Status(context.Background())
			require.NoError(t, err)
			require.Len(t, snapshot.Active, 1)
			peak = snapshot.Active[0].PeakBytes
			require.Positive(t, peak)
			require.LessOrEqual(t, peak, int64(8<<20), "回归不应写入大型测试文件")
			files, err := filepath.Glob(filepath.Join(downloads, "powerx-ai-media-*"))
			require.NoError(t, err)
			require.Len(t, files, 1)
			temporary = files[0]
		}
		if written == peak {
			policy, err := c.Policy(context.Background())
			require.NoError(t, err)
			policy.MinFreeBytes = 1 << 50
			_, err = c.SavePolicy(context.Background(), policy.Revision, policy)
			require.NoError(t, err)
			return copy(p, "must not grow without a reservation"), io.EOF
		}
		n := min(int64(len(p)), peak-written)
		clear(p[:n])
		written += n
		return int(n), nil
	})
	_, err := (&MediaRuntime{}).storeOutput(context.Background(), "tenant", "task", 0, "image", body)
	require.ErrorIs(t, err, budget.ErrInsufficient)
	require.Equal(t, peak, written)
	require.NoFileExists(t, temporary)
	requireMediaOutputReleased(t, c, downloads)
}

func TestMediaOutputAdmissionSharesLeaseThroughLocalTransfer(t *testing.T) {
	f := newMediaFixture(t)
	c, downloads := mediaOutputBudgetFixture(t)
	task := uuid.NewString()
	source := bytes.NewReader(f.png)
	body := mediaOutputReaderFunc(func(p []byte) (int, error) {
		snapshot, err := c.Status(context.Background())
		require.NoError(t, err)
		require.Len(t, snapshot.Active, 1)
		return source.Read(p)
	})
	asset, err := f.s.storeOutput(f.ctx, mediaTestTenant, task, 0, "image", body)
	require.NoError(t, err, "并发上限为 1 时同盘正式转存不能与自己的临时租约竞争")
	requireMediaOutputReleased(t, c, downloads)
	_, object, err := f.media.OpenAssetResource(f.ctx, mediaTestTenant, asset.MediaAssetUUID)
	require.NoError(t, err)
	defer object.Body.Close()
	stored, err := io.ReadAll(object.Body)
	require.NoError(t, err)
	require.Equal(t, f.png, stored)
	again, err := f.s.storeOutput(f.ctx, mediaTestTenant, task, 0, "image", bytes.NewReader(f.png))
	require.NoError(t, err)
	require.Equal(t, asset.MediaAssetUUID, again.MediaAssetUUID)
	requireMediaOutputReleased(t, c, downloads)
}

func TestMediaOutputAdmissionDefersTransferWithoutRegenerating(t *testing.T) {
	f := newMediaFixture(t)
	task, err := f.s.Submit(f.ctx, videoInput())
	require.NoError(t, err)
	f.consume(t, f.s)
	require.EqualValues(t, 1, f.generations.Load())
	c, downloads := mediaOutputBudgetFixture(t)
	policy, err := c.Policy(context.Background())
	require.NoError(t, err)
	policy.MinFreeBytes = 1 << 50
	_, err = c.SavePolicy(context.Background(), policy.Revision, policy)
	require.NoError(t, err)
	f.consume(t, f.s)
	state, err := f.s.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "running", state.Status)
	require.Equal(t, "materializing", state.Phase)
	require.Equal(t, "AI_MEDIA_STORAGE_CAPACITY_EXHAUSTED", state.ErrorCode)
	require.Empty(t, state.Assets)
	require.EqualValues(t, 1, f.generations.Load(), "容量拒绝不能重新提交供应商生成")
	requireMediaOutputReleased(t, c, downloads)
}

func TestCopyMediaOutputPreservesSizeLimitAndChecksum(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{name: "empty"},
		{name: "at_limit", body: "12345678", valid: true},
		{name: "over_limit", body: "123456789more bytes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var destination bytes.Buffer
			size, checksum, err := copyMediaOutput(context.Background(), &destination, bytes.NewBufferString(test.body), 8)
			require.LessOrEqual(t, size, int64(9))
			if !test.valid {
				requireMediaCode(t, err, "AI_MEDIA_SIZE_INVALID")
				require.Empty(t, checksum)
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, len(test.body), size)
			sum := sha256.Sum256([]byte(test.body))
			require.Equal(t, hex.EncodeToString(sum[:]), checksum)
			require.Equal(t, test.body, destination.String())
		})
	}
}

func TestMediaOutputAdmissionClassifiesUnavailableStorage(t *testing.T) {
	for _, err := range []error{budget.ErrUnavailable, budget.ErrDeviceChanged, budget.ErrControlRequired, budget.ErrControlChanged} {
		require.Equal(t, "AI_MEDIA_STORAGE_UNAVAILABLE", mediaErrorCode(err))
	}
}
