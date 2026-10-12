package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type variantMemoryDriver struct {
	stubStorageDriver
	content  string
	mime     string
	putError error
	getBody  io.ReadCloser
}

func (d *variantMemoryDriver) Put(_ context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
	b, e := io.ReadAll(in.Body)
	d.content = string(b)
	d.mime = in.ContentType
	if d.putError != nil {
		return nil, d.putError
	}
	return &driver.PutObjectResult{Size: int64(len(b))}, e
}
func (d *variantMemoryDriver) Get(_ context.Context, _ driver.GetObjectInput) (*driver.GetObjectResult, error) {
	if d.getBody != nil {
		return &driver.GetObjectResult{Body: d.getBody, Size: 7, ContentType: d.mime}, nil
	}
	return &driver.GetObjectResult{Body: io.NopCloser(strings.NewReader(d.content)), Size: int64(len(d.content)), ContentType: d.mime}, nil
}

func TestVariantPutFailureRevokesTicket(t *testing.T) {
	s, r, d, a, v, _ := variantFixture(t)
	ticket, err := s.IssueVariantTransferTicket(context.Background(), mediaTenantUUID, a, v, "upload", time.Minute)
	require.NoError(t, err)
	d.putError = errors.New("storage_write_failed")
	_, err = consumeVariant(s, ticket, "payload")
	require.ErrorIs(t, err, d.putError)
	require.Equal(t, m.UploadStateFailed, r.variants[a+"|preview"].UploadState)
	require.Nil(t, r.variants[a+"|preview"].UploadExpiresAt)
	_, err = consumeVariant(s, ticket, "payload")
	require.ErrorIs(t, err, ErrUploadTicketExpired)
}

func TestVariantPutChecksActualLength(t *testing.T) {
	s, r, _, a, v, _ := variantFixture(t)
	// A matching checksum must not override the declared size invariant.
	hash := sha256.Sum256([]byte("short"))
	r.variants[a+"|preview"].ExpectedChecksum = hex.EncodeToString(hash[:])
	ticket, err := s.IssueVariantTransferTicket(context.Background(), mediaTenantUUID, a, v, "upload", time.Minute)
	require.NoError(t, err)
	_, err = s.ConsumeVariantTransfer(context.Background(), a, v, "upload", strconv.FormatInt(ticket.ExpiresAt.Unix(), 10), "1", "1", ticket.Token, strings.NewReader("short"), 7, ticket.MimeType)
	require.ErrorIs(t, err, ErrUploadValidationFailed)
	require.Equal(t, m.UploadStateFailed, r.variants[a+"|preview"].UploadState)
}

type variantCommitFailureRepo struct {
	*stubAssetRepo
	failure error
}

func (r *variantCommitFailureRepo) WithVariantTransfer(ctx context.Context, tenant, asset, variant string, apply func(*m.MediaAsset, *m.MediaAssetVariant) error) error {
	if err := r.stubAssetRepo.WithVariantTransfer(ctx, tenant, asset, variant, apply); err != nil {
		return err
	}
	return r.failure
}

type variantTrackedBody struct {
	io.Reader
	closed bool
}

func (b *variantTrackedBody) Close() error { b.closed = true; return nil }

func TestVariantDownloadClosesBodyOnCommitFailure(t *testing.T) {
	s, r, d, a, v, _ := variantFixture(t)
	r.variants[a+"|preview"].UploadState = m.UploadStateReady
	ticket, err := s.IssueVariantTransferTicket(context.Background(), mediaTenantUUID, a, v, "download", time.Minute)
	require.NoError(t, err)
	failure := errors.New("transaction_commit_failed")
	s.repo = &variantCommitFailureRepo{stubAssetRepo: r, failure: failure}
	body := &variantTrackedBody{Reader: strings.NewReader("payload")}
	d.getBody = body
	result, err := consumeVariant(s, ticket, "")
	require.ErrorIs(t, err, failure)
	require.Nil(t, result)
	require.True(t, body.closed)
}
func variantFixture(t *testing.T) (*MediaService, *stubAssetRepo, *variantMemoryDriver, string, string, string) {
	t.Helper()
	repo := newStubAssetRepo()
	asset := uuid.NewString()
	variant := uuid.NewString()
	hash := sha256.Sum256([]byte("payload"))
	checksum := hex.EncodeToString(hash[:])
	expires := time.Now().Add(time.Hour)
	repo.assets[asset] = &m.MediaAsset{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(asset)}, TenantUUID: mediaTenantUUID, UploadState: m.UploadStateReady, UploadTicketVersion: 1}
	repo.variants[asset+"|preview"] = &m.MediaAssetVariant{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(variant)}, TenantUUID: mediaTenantUUID, AssetUUID: asset, Variant: "preview", Driver: "local", StorageKey: asset + "/preview", SizeBytes: 7, MimeType: "text/plain", ExpectedChecksum: checksum, UploadState: m.UploadStatePending, UploadExpiresAt: &expires, TicketVersion: 1}
	d := &variantMemoryDriver{}
	manager := mediamgr.New("local")
	manager.RegisterDriver(d)
	svc := NewMediaService(nil, repo, manager, &stubAuditService{}, time.Hour)
	svc.publicResourceTokenSecret = []byte("variant-test-secret")
	return svc, repo, d, asset, variant, checksum
}
func consumeVariant(s *MediaService, ticket *VariantTransferTicket, body string) (*driver.GetObjectResult, error) {
	return s.ConsumeVariantTransfer(context.Background(), ticket.AssetUUID, ticket.VariantUUID, ticket.Action, strconv.FormatInt(ticket.ExpiresAt.Unix(), 10), strconv.FormatUint(ticket.Version, 10), strconv.FormatUint(ticket.ParentVersion, 10), ticket.Token, strings.NewReader(body), int64(len(body)), ticket.MimeType)
}
func TestVariantTransferLifecycleAndRevocation(t *testing.T) {
	s, r, _, a, v, checksum := variantFixture(t)
	ctx := context.Background()
	_, e := s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "download", time.Minute)
	require.ErrorIs(t, e, ErrInvalidStatusTransition)
	_, e = s.CompleteVariantUpload(ctx, mediaTenantUUID, a, v, checksum)
	require.ErrorIs(t, e, ErrInvalidStatusTransition)
	ticket, e := s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "upload", time.Minute)
	require.NoError(t, e)
	_, e = consumeVariant(s, ticket, "payload")
	require.NoError(t, e)
	require.Equal(t, m.UploadStateUploaded, r.variants[a+"|preview"].UploadState)
	_, e = consumeVariant(s, ticket, "payload")
	require.ErrorIs(t, e, ErrUploadTicketExpired)
	result, e := s.CompleteVariantUpload(ctx, mediaTenantUUID, a, v, checksum)
	require.NoError(t, e)
	require.Equal(t, m.UploadStateReady, result.UploadState)
	_, e = s.CompleteVariantUpload(ctx, mediaTenantUUID, a, v, checksum)
	require.NoError(t, e)
	download, e := s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "download", time.Minute)
	require.NoError(t, e)
	object, e := consumeVariant(s, download, "")
	require.NoError(t, e)
	b, e := io.ReadAll(object.Body)
	require.NoError(t, e)
	object.Body.Close()
	require.Equal(t, "payload", string(b))
	r.assets[a].UploadState = m.UploadStateDeleted
	r.assets[a].UploadTicketVersion++
	_, e = consumeVariant(s, download, "")
	require.ErrorIs(t, e, ErrUploadTicketExpired)
}
func TestVariantTransferIsolationAndValidation(t *testing.T) {
	s, r, d, a, v, checksum := variantFixture(t)
	ctx := context.Background()
	_, e := s.IssueVariantTransferTicket(ctx, uuid.NewString(), a, v, "upload", time.Minute)
	require.ErrorIs(t, e, ErrAssetNotFound)
	_, e = s.IssueVariantTransferTicket(ctx, mediaTenantUUID, uuid.NewString(), v, "upload", time.Minute)
	require.ErrorIs(t, e, ErrAssetNotFound)
	for _, ttl := range []time.Duration{59 * time.Second, 3601 * time.Second} {
		_, e = s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "upload", ttl)
		require.Error(t, e)
	}
	ticket, e := s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "upload", time.Minute)
	require.NoError(t, e)
	forged := *ticket
	forged.VariantUUID = uuid.NewString()
	_, e = consumeVariant(s, &forged, "payload")
	require.ErrorIs(t, e, ErrUploadTicketExpired)
	forged = *ticket
	forged.Action = "download"
	_, e = consumeVariant(s, &forged, "")
	require.ErrorIs(t, e, ErrUploadTicketExpired)
	_, e = consumeVariant(s, ticket, "payload")
	require.NoError(t, e)
	d.content = "changed"
	_, e = s.CompleteVariantUpload(ctx, mediaTenantUUID, a, v, checksum)
	require.ErrorIs(t, e, ErrUploadValidationFailed)
	require.Equal(t, m.UploadStateFailed, r.variants[a+"|preview"].UploadState)
	_, e = s.IssueVariantTransferTicket(ctx, mediaTenantUUID, a, v, "download", time.Minute)
	require.ErrorIs(t, e, ErrInvalidStatusTransition)
}
