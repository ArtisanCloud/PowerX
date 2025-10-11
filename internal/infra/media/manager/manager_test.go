package manager

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
)

// fakeDriver 用于模拟不同操作的驱动实现。
type fakeDriver struct {
	name     string
	putFn    func(ctx context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error)
	getFn    func(ctx context.Context, in driver.GetObjectInput) (*driver.GetObjectResult, error)
	deleteFn func(ctx context.Context, in driver.DeleteObjectInput) error
	urlFn    func(ctx context.Context, in driver.GenerateURLInput) (*driver.GenerateURLOutput, error)
	healthFn func(ctx context.Context) error
}

func (f *fakeDriver) Name() string { return f.name }

func (f *fakeDriver) Put(ctx context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
	if f.putFn != nil {
		return f.putFn(ctx, in)
	}
	return &driver.PutObjectResult{Bucket: in.Bucket, ObjectKey: in.ObjectKey, Size: in.Size}, nil
}

func (f *fakeDriver) Get(ctx context.Context, in driver.GetObjectInput) (*driver.GetObjectResult, error) {
	if f.getFn != nil {
		return f.getFn(ctx, in)
	}
	return &driver.GetObjectResult{Bucket: in.Bucket, ObjectKey: in.ObjectKey, Size: 1}, nil
}

func (f *fakeDriver) Delete(ctx context.Context, in driver.DeleteObjectInput) error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, in)
	}
	return nil
}

func (f *fakeDriver) GenerateURL(ctx context.Context, in driver.GenerateURLInput) (*driver.GenerateURLOutput, error) {
	if f.urlFn != nil {
		return f.urlFn(ctx, in)
	}
	return &driver.GenerateURLOutput{Bucket: in.Bucket, ObjectKey: in.ObjectKey, Method: in.Method, ExpireAt: time.Now().Add(in.TTL)}, nil
}

func (f *fakeDriver) HealthCheck(ctx context.Context) error {
	if f.healthFn != nil {
		return f.healthFn(ctx)
	}
	return nil
}

func TestMediaManager_RegisterAndDefault(t *testing.T) {
	manager := New("local")
	manager.RegisterDriver(&fakeDriver{name: "local"})
	require.NoError(t, manager.SetDefaultDriver("local"))

	defaultName, err := manager.DefaultDriver()
	require.NoError(t, err)
	assert.Equal(t, "local", defaultName)

	drivers := manager.Drivers()
	require.Len(t, drivers, 1)
	assert.Equal(t, "local", drivers[0])
}

func TestMediaManager_PutUsesDefaultDriver(t *testing.T) {
	ctx := context.Background()
	manager := New("local")
	var received driver.PutObjectInput
	manager.RegisterDriver(&fakeDriver{
		name: "local",
		putFn: func(_ context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
			received = in
			return &driver.PutObjectResult{Bucket: in.Bucket, ObjectKey: in.ObjectKey, Size: in.Size}, nil
		},
	})
	require.NoError(t, manager.SetDefaultDriver("local"))

	reader := bytes.NewBufferString("hello")
	result, err := manager.Put(ctx, "", driver.PutObjectInput{Bucket: "demo", ObjectKey: "key.txt", Body: reader, Size: int64(reader.Len()), Overwrite: true})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "demo", received.Bucket)
	assert.Equal(t, "key.txt", received.ObjectKey)
	assert.Equal(t, int64(reader.Len()), received.Size)

	snapshot := manager.MetricsSnapshot()
	require.Len(t, snapshot, 1)
	assert.Contains(t, snapshot[0].Metrics, string(operationPut))
}

func TestMediaManager_DeletePropagatesError(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("boom")
	manager := New("local")
	manager.RegisterDriver(&fakeDriver{
		name: "local",
		deleteFn: func(context.Context, driver.DeleteObjectInput) error {
			return expected
		},
	})
	require.NoError(t, manager.SetDefaultDriver("local"))

	err := manager.Delete(ctx, "local", driver.DeleteObjectInput{Bucket: "demo", ObjectKey: "missing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, expected))

	snapshot := manager.MetricsSnapshot()
	require.Len(t, snapshot, 1)
	metrics := snapshot[0].Metrics[string(operationDelete)]
	assert.Equal(t, int64(1), metrics.Calls)
	assert.Equal(t, int64(1), metrics.Failures)
}
