package plugin_release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
)

type server struct {
	pluginreleasepb.UnimplementedPluginReleaseServiceServer
	svc *svc.Service
}

// RegisterServer wires plugin release gRPC handlers.
func RegisterServer(registrar grpc.ServiceRegistrar, deps *shared.Deps) {
	if registrar == nil || deps == nil || deps.PluginReleaseService == nil {
		return
	}
	pluginreleasepb.RegisterPluginReleaseServiceServer(registrar, &server{
		svc: deps.PluginReleaseService,
	})
}

func (s *server) StartLocalInstall(ctx context.Context, req *pluginreleasepb.StartLocalInstallRequest) (*pluginreleasepb.LocalInstallSession, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}

	tenantID, err := parseTenantID(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	session, err := localSvc.Start(ctx, local.StartInput{
		TenantID:     tenantID,
		DeveloperID:  req.GetDeveloperId(),
		ArtifactURI:  req.GetArtifactUri(),
		FeatureFlags: req.GetFeatureFlags(),
		ResetCache:   req.GetResetCache(),
		Actor:        actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapLocalError(err)
	}

	return toProtoSession(session), nil
}

func (s *server) StopLocalInstall(ctx context.Context, req *pluginreleasepb.StopLocalInstallRequest) (*pluginreleasepb.StopLocalInstallResponse, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}
	sessionUUID, err := parseSessionID(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := localSvc.Stop(ctx, local.StopInput{
		SessionID: sessionUUID,
		Force:     req.GetForce(),
		Actor:     actorFromContext(ctx),
	}); err != nil {
		return nil, mapLocalError(err)
	}

	targetStatus := models.LocalInstallStatusSuccess
	if req.GetForce() {
		targetStatus = models.LocalInstallStatusFailed
	}

	return &pluginreleasepb.StopLocalInstallResponse{
		SessionId: req.GetSessionId(),
		Status:    targetStatus,
	}, nil
}

func (s *server) GetLocalInstallSession(ctx context.Context, req *pluginreleasepb.GetLocalInstallSessionRequest) (*pluginreleasepb.LocalInstallSession, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}
	sessionUUID, err := parseSessionID(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	session, err := localSvc.Get(ctx, sessionUUID)
	if err != nil {
		return nil, mapLocalError(err)
	}

	if tenant := strings.TrimSpace(req.GetTenantId()); tenant != "" {
		if tenant != strconv.FormatUint(session.TenantID, 10) {
			return nil, status.Error(codes.NotFound, "local install session not found")
		}
	}

	return toProtoSession(session), nil
}

func (s *server) PushHotReload(stream pluginreleasepb.PluginReleaseService_PushHotReloadServer) error {
	localSvc := s.localInstall()
	if localSvc == nil {
		return status.Error(codes.Unavailable, "local install service unavailable")
	}

	ctx := stream.Context()
	var (
		sessionUUID uuid.UUID
		sessionID   string
		lastSeq     int64
		ackStatus   = "accepted"
	)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			if sessionID == "" {
				return status.Error(codes.InvalidArgument, "no hot reload chunks received")
			}
			return stream.SendAndClose(&pluginreleasepb.HotReloadAck{
				SessionId:       sessionID,
				AppliedSequence: lastSeq,
				Status:          ackStatus,
				Message:         "",
			})
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive chunk failed: %v", err)
		}

		chunkSession := strings.TrimSpace(chunk.GetSessionId())
		if chunkSession == "" {
			return status.Error(codes.InvalidArgument, "session_id is required in stream")
		}
		if sessionID == "" {
			sessionID = chunkSession
			var parseErr error
			sessionUUID, parseErr = uuid.Parse(sessionID)
			if parseErr != nil {
				return status.Error(codes.InvalidArgument, "invalid session_id")
			}
			if _, err := localSvc.Get(ctx, sessionUUID); err != nil {
				return mapLocalError(err)
			}
		} else if sessionID != chunkSession {
			return status.Error(codes.InvalidArgument, "session_id mismatch within stream")
		}

		lastSeq = chunk.GetSequence()
		pointers := map[string]any{
			"hotreload_last_sequence": lastSeq,
			"hotreload_updated_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}
		if chunk.GetChangelog() != "" {
			pointers["hotreload_changelog"] = chunk.GetChangelog()
		}
		if err := localSvc.UpdateLogPointers(ctx, sessionUUID, pointers); err != nil {
			return mapLocalError(err)
		}
		if chunk.GetEof() {
			ackStatus = "completed"
		}
	}
}

func (s *server) localInstall() *local.InstallService {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.LocalInstall()
}

func parseTenantID(raw string) (uint64, error) {
	tenant := strings.TrimSpace(raw)
	if tenant == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	value, err := strconv.ParseUint(tenant, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("tenant_id must be numeric")
	}
	return value, nil
}

func parseSessionID(value string) (uuid.UUID, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return uuid.Nil, fmt.Errorf("session_id is required")
	}
	sessionUUID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid session_id")
	}
	return sessionUUID, nil
}

func actorFromContext(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("authorization"); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func mapLocalError(err error) error {
	switch {
	case errors.Is(err, local.ErrFeatureDisabled):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, local.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, local.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, local.ErrSignatureInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, local.ErrActiveSession):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, local.ErrArtifactTooLarge):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, local.ErrSessionNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		logger.WarnF(context.Background(), "local install operation failed: %v", err)
		return status.Error(codes.Internal, "local install operation failed")
	}
}

func toProtoSession(session *models.LocalInstallSession) *pluginreleasepb.LocalInstallSession {
	if session == nil {
		return nil
	}
	resp := &pluginreleasepb.LocalInstallSession{
		SessionId:   session.UUID.String(),
		TenantId:    strconv.FormatUint(session.TenantID, 10),
		DeveloperId: session.DeveloperID,
		ArtifactUri: session.ArtifactURI,
		FeatureFlags: func() []string {
			flags := local.ExtractFeatureFlags(session.FeatureFlags)
			if flags == nil {
				return []string{}
			}
			return flags
		}(),
		Status: session.Status,
		LogUrl: extractLogURL(session.LogPointers),
	}
	if !session.CreatedAt.IsZero() {
		resp.CreatedAt = session.CreatedAt.UTC().Format(time.RFC3339)
	}
	if session.ExpiredAt != nil {
		resp.ExpiresAt = session.ExpiredAt.UTC().Format(time.RFC3339)
	}
	return resp
}

func extractLogURL(raw datatypes.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if v, ok := payload["log_url"].(string); ok && v != "" {
		return v
	}
	if v, ok := payload["logUrl"].(string); ok && v != "" {
		return v
	}
	return ""
}
