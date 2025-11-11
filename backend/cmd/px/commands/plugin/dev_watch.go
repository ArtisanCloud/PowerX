package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	defaultGRPCAddr      = "localhost:9090"
	defaultChunkSize     = 256 * 1024 // 256 KiB
	defaultLogTailLines  = 200
	maxLogTailBytes      = 64 * 1024
	maxChangelogRuneSize = 8 * 1024
)

var (
	devCmd = &cobra.Command{
		Use:   "dev",
		Short: "Developer utilities for plugin hotload",
	}

	devWatchOpts = struct {
		grpcAddr     string
		hostAPI      string
		tenantID     string
		developerID  uint64
		artifactPath string
		artifactURI  string
		featureFlags []string
		resetCache   bool
		token        string
		stop         bool
		chunkSize    int
		timeout      time.Duration
		changelog    string
		logFile      string
		logLines     int
	}{
		stop:      true,
		chunkSize: defaultChunkSize,
		timeout:   30 * time.Second,
		logLines:  defaultLogTailLines,
	}

	devWatchCmd = &cobra.Command{
		Use:   "watch",
		Short: "Push a hotload bundle to a tenant session",
		Long:  "Establish a local install session via gRPC, push artifact chunks, and optionally close the session once the update is applied.",
		RunE:  runDevWatch,
	}
)

func init() {
	Command.AddCommand(devCmd)
	devCmd.AddCommand(devWatchCmd)

	devWatchCmd.Flags().StringVar(&devWatchOpts.grpcAddr, "grpc-addr", defaultGRPCAddr, "Plugin release gRPC endpoint")
	devWatchCmd.Flags().StringVar(&devWatchOpts.hostAPI, "host-api", "", "Optional PowerX Admin API base (e.g., http://localhost:8077/api) used for local install REST endpoints")
	devWatchCmd.Flags().StringVar(&devWatchOpts.tenantID, "tenant-id", "", "Tenant identifier used during local install (required)")
	devWatchCmd.Flags().Uint64Var(&devWatchOpts.developerID, "developer-id", 0, "Developer identifier used for the session (required)")
	devWatchCmd.Flags().StringVar(&devWatchOpts.artifactPath, "artifact", "", "Path to the hotload artifact to stream via PushHotReload")
	devWatchCmd.Flags().StringVar(&devWatchOpts.artifactURI, "artifact-uri", "", "Override artifact URI sent in StartLocalInstall; defaults to derived file:// URI when --artifact is supplied")
	devWatchCmd.Flags().StringSliceVar(&devWatchOpts.featureFlags, "feature-flag", nil, "Feature flags to enable during the hotload session")
	devWatchCmd.Flags().BoolVar(&devWatchOpts.resetCache, "reset-cache", false, "Request backend cache reset before applying the hotload bundle")
	devWatchCmd.Flags().StringVar(&devWatchOpts.token, "token", "", "Bearer token used for gRPC metadata Authorization header")
	devWatchCmd.Flags().BoolVar(&devWatchOpts.stop, "stop", true, "Automatically stop the local install session after a successful push")
	devWatchCmd.Flags().IntVar(&devWatchOpts.chunkSize, "chunk-size", defaultChunkSize, "Hot reload stream chunk size in bytes")
	devWatchCmd.Flags().DurationVar(&devWatchOpts.timeout, "timeout", devWatchOpts.timeout, "Overall RPC timeout")
	devWatchCmd.Flags().StringVar(&devWatchOpts.changelog, "changelog", "", "Explicit changelog/log snippet to attach to the session (takes precedence over --log-file)")
	devWatchCmd.Flags().StringVar(&devWatchOpts.logFile, "log-file", "", "Path to a log file whose tail will be attached as changelog (use - for stdin)")
	devWatchCmd.Flags().IntVar(&devWatchOpts.logLines, "log-lines", devWatchOpts.logLines, "Number of lines to include from --log-file (0 to disable)")

	_ = devWatchCmd.MarkFlagRequired("tenant-id")
	_ = devWatchCmd.MarkFlagRequired("developer-id")
}

func runDevWatch(cmd *cobra.Command, args []string) error {
	if devWatchOpts.chunkSize <= 0 {
		return errors.New("chunk-size must be positive")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), devWatchOpts.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, devWatchOpts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gRPC: %w", err)
	}
	defer conn.Close()

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)

	callCtx := attachAuth(ctx, devWatchOpts.token)

	artifactURI, err := resolveArtifactURI(devWatchOpts.artifactURI, devWatchOpts.artifactPath)
	if err != nil {
		return err
	}

	changelog, err := collectChangelog(cmd)
	if err != nil {
		return fmt.Errorf("collect changelog: %w", err)
	}

	session, err := startLocalSession(cmd, callCtx, client, artifactURI)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Session %s started for tenant %s developer %d\n", session.SessionID, devWatchOpts.tenantID, devWatchOpts.developerID)

	reloadStart := time.Now()
	sequence, reloadErr := pushHotReload(callCtx, client, session.SessionID, devWatchOpts.artifactPath, devWatchOpts.chunkSize, changelog, cmd.OutOrStdout())
	reloadDuration := time.Since(reloadStart)
	if hostAPIEnabled() {
		recordReloadEvent(callCtx, session.SessionID, sequence, reloadDuration, reloadErr)
	}
	if reloadErr != nil {
		return fmt.Errorf("push hot reload: %w", reloadErr)
	}

	sessionInfo, err := client.GetLocalInstallSession(callCtx, &pluginreleasepb.GetLocalInstallSessionRequest{
		SessionId: session.SessionID,
	})
	if err == nil {
		printSessionSummary(cmd, "Session status", sessionInfo)
	} else {
		logger.WarnF(callCtx, "fetch session status failed: %v", err)
	}

	if devWatchOpts.stop {
		stopResp, err := client.StopLocalInstall(callCtx, &pluginreleasepb.StopLocalInstallRequest{
			SessionId: session.SessionID,
		})
		if err != nil {
			return fmt.Errorf("stop local install: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Session %s closed with status %s\n", stopResp.GetSessionId(), stopResp.GetStatus())
	}

	return nil
}

func pushHotReload(ctx context.Context, client pluginreleasepb.PluginReleaseServiceClient, sessionID, artifactPath string, chunkSize int, changelog string, out io.Writer) (int64, error) {
	stream, err := client.PushHotReload(ctx)
	if err != nil {
		return 0, err
	}

	var (
		sequence         int64 = 1
		sentAny                = false
		appliedChangelog       = false
		trimmedChangelog       = truncateChangelog(strings.TrimSpace(changelog))
	)

	if strings.TrimSpace(artifactPath) != "" {
		file, err := os.Open(artifactPath)
		if err != nil {
			return 0, fmt.Errorf("open artifact: %w", err)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return 0, fmt.Errorf("stat artifact: %w", err)
		}

		totalSize := info.Size()
		var totalRead int64
		reader := bufio.NewReader(file)
		buf := make([]byte, chunkSize)

		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				totalRead += int64(n)
				chunk := &pluginreleasepb.HotReloadChunk{
					SessionId: sessionID,
					Sequence:  sequence,
					Content:   buf[:n],
				}
				sequence++
				sentAny = true

				if totalSize > 0 && totalRead >= totalSize {
					chunk.Eof = true
				}

				if chunk.Eof && trimmedChangelog != "" && !appliedChangelog {
					chunk.Changelog = trimmedChangelog
					appliedChangelog = true
				}

				if err := stream.Send(chunk); err != nil {
					return 0, fmt.Errorf("send chunk: %w", err)
				}

				if chunk.Eof {
					break
				}
			}

			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return 0, fmt.Errorf("read artifact: %w", readErr)
			}
			if n == 0 {
				break
			}
		}
	}

	if !sentAny {
		chunk := &pluginreleasepb.HotReloadChunk{
			SessionId: sessionID,
			Sequence:  sequence,
			Eof:       true,
		}
		if trimmedChangelog != "" {
			chunk.Changelog = trimmedChangelog
			appliedChangelog = true
		}
		if err := stream.Send(chunk); err != nil {
			return 0, fmt.Errorf("send empty chunk: %w", err)
		}
		sequence++
	} else if trimmedChangelog != "" && !appliedChangelog {
		if err := stream.Send(&pluginreleasepb.HotReloadChunk{
			SessionId: sessionID,
			Sequence:  sequence,
			Changelog: trimmedChangelog,
		}); err != nil {
			return 0, fmt.Errorf("send changelog chunk: %w", err)
		}
		appliedChangelog = true
		sequence++
	}

	ack, err := stream.CloseAndRecv()
	if err != nil {
		return 0, fmt.Errorf("close hot reload stream: %w", err)
	}

	fmt.Fprintf(out, "Hot reload applied (seq=%d status=%s)\n", ack.GetAppliedSequence(), ack.GetStatus())
	return ack.GetAppliedSequence(), nil
}

func resolveArtifactURI(explicitURI, artifactPath string) (string, error) {
	if strings.TrimSpace(explicitURI) != "" {
		return explicitURI, nil
	}
	if strings.TrimSpace(artifactPath) == "" {
		return "", nil
	}
	abs, err := filepath.Abs(artifactPath)
	if err != nil {
		return "", fmt.Errorf("resolve artifact path: %w", err)
	}
	return "file://" + filepath.ToSlash(abs), nil
}

func attachAuth(ctx context.Context, token string) context.Context {
	token = strings.TrimSpace(token)
	if token == "" {
		return ctx
	}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", token)
}

func printSessionSummary(cmd *cobra.Command, title string, session *pluginreleasepb.LocalInstallSession) {
	if session == nil {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title)
	fmt.Fprintf(cmd.OutOrStdout(), "  Session: %s\n", session.GetSessionId())
	fmt.Fprintf(cmd.OutOrStdout(), "  Status : %s\n", session.GetStatus())
	if session.GetLogUrl() != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Log URL: %s\n", session.GetLogUrl())
	}
}

func collectChangelog(cmd *cobra.Command) (string, error) {
	if cmd == nil {
		return "", nil
	}

	if msg := strings.TrimSpace(devWatchOpts.changelog); msg != "" {
		return truncateChangelog(msg), nil
	}

	path := strings.TrimSpace(devWatchOpts.logFile)
	if path == "" {
		return "", nil
	}

	lines := devWatchOpts.logLines
	if lines <= 0 {
		return "", nil
	}

	var (
		content string
		err     error
	)

	if path == "-" {
		content, err = tailFromReader(cmd.InOrStdin(), lines)
	} else {
		content, err = tailFile(path, lines)
	}
	if err != nil {
		return "", err
	}
	return truncateChangelog(content), nil
}

func tailFile(path string, lines int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	offset := size - maxLogTailBytes
	if offset < 0 {
		offset = 0
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return "", err
		}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return extractTail(string(data), lines), nil
}

func tailFromReader(r io.Reader, lines int) (string, error) {
	if r == nil {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(r, maxLogTailBytes))
	if err != nil {
		return "", err
	}
	return extractTail(string(data), lines), nil
}

func extractTail(content string, lines int) string {
	if lines <= 0 {
		return ""
	}
	segments := strings.Split(content, "\n")
	if len(segments) > lines {
		segments = segments[len(segments)-lines:]
	}
	return strings.TrimSpace(strings.Join(segments, "\n"))
}

func truncateChangelog(content string) string {
	if content == "" {
		return ""
	}
	if len(content) <= maxChangelogRuneSize {
		return content
	}

	var builder strings.Builder
	builder.Grow(maxChangelogRuneSize)
	for _, r := range content {
		size := utf8.RuneLen(r)
		if builder.Len()+size > maxChangelogRuneSize-3 {
			builder.WriteString("...")
			break
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

type localSession struct {
	SessionID string
	TenantID  uint64
	LogURL    string
}

func startLocalSession(cmd *cobra.Command, ctx context.Context, client pluginreleasepb.PluginReleaseServiceClient, artifactURI string) (*localSession, error) {
	if hostAPIEnabled() {
		return startSessionViaHTTP(ctx, artifactURI)
	}
	startResp, err := client.StartLocalInstall(ctx, &pluginreleasepb.StartLocalInstallRequest{
		TenantId:     strings.TrimSpace(devWatchOpts.tenantID),
		DeveloperId:  devWatchOpts.developerID,
		ArtifactUri:  artifactURI,
		FeatureFlags: devWatchOpts.featureFlags,
		ResetCache:   devWatchOpts.resetCache,
	})
	if err != nil {
		return nil, fmt.Errorf("start local install: %w", err)
	}
	tenantNumeric, _ := strconv.ParseUint(strings.TrimSpace(devWatchOpts.tenantID), 10, 64)
	return &localSession{
		SessionID: startResp.GetSessionId(),
		TenantID:  tenantNumeric,
		LogURL:    startResp.GetLogUrl(),
	}, nil
}

func startSessionViaHTTP(ctx context.Context, artifactURI string) (*localSession, error) {
	tenantNumeric, err := strconv.ParseUint(strings.TrimSpace(devWatchOpts.tenantID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("tenant-id must be numeric when --host-api is set: %w", err)
	}
	payload := localInstallHTTPRequest{
		TenantID:     tenantNumeric,
		DeveloperID:  devWatchOpts.developerID,
		ArtifactURI:  artifactURI,
		FeatureFlags: devWatchOpts.featureFlags,
		ResetCache:   devWatchOpts.resetCache,
	}
	var resp localInstallHTTPResponse
	if err := doHostAPIRequest(ctx, http.MethodPost, "/internal/plugins/local/install", payload, &resp); err != nil {
		return nil, err
	}
	if resp.Err != "" {
		return nil, errors.New(resp.Err)
	}
	if resp.Data.SessionID == "" {
		return nil, errors.New("host API returned empty session id")
	}
	return &localSession{
		SessionID: resp.Data.SessionID,
		TenantID:  resp.Data.TenantID,
		LogURL:    resp.Data.LogURL,
	}, nil
}

func recordReloadEvent(ctx context.Context, sessionID string, sequence int64, duration time.Duration, reloadErr error) {
	payload := localReloadPayload{
		SessionID:       sessionID,
		DurationMs:      duration.Milliseconds(),
		Sequence:        sequence,
		Success:         reloadErr == nil,
		VersionMismatch: isVersionMismatchError(reloadErr),
	}
	if reloadErr != nil {
		payload.Error = reloadErr.Error()
	}
	if err := doHostAPIRequest(ctx, http.MethodPost, "/internal/plugins/local/reload", payload, nil); err != nil {
		logger.WarnF(ctx, "record reload event failed: %v", err)
	}
}

func hostAPIEnabled() bool {
	return strings.TrimSpace(devWatchOpts.hostAPI) != ""
}

func doHostAPIRequest(ctx context.Context, method, path string, payload any, dest any) error {
	if !hostAPIEnabled() {
		return errors.New("host API not configured")
	}
	base := strings.TrimRight(devWatchOpts.hostAPI, "/")
	if base == "" {
		return errors.New("invalid host API base")
	}
	url := base + path
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token := strings.TrimSpace(devWatchOpts.token)
	if token != "" {
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}
	client := &http.Client{Timeout: devWatchOpts.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("host API %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(data, dest)
}

type localInstallHTTPRequest struct {
	TenantID     uint64   `json:"tenantId"`
	DeveloperID  uint64   `json:"developerId"`
	ArtifactURI  string   `json:"artifactUri"`
	FeatureFlags []string `json:"featureFlags"`
	ResetCache   bool     `json:"resetCache"`
}

type localInstallHTTPResponse struct {
	Data localInstallHTTPData `json:"data"`
	Err  string               `json:"error"`
}

type localInstallHTTPData struct {
	SessionID string `json:"sessionId"`
	TenantID  uint64 `json:"tenantId"`
	LogURL    string `json:"logUrl"`
}

type localReloadPayload struct {
	SessionID       string `json:"sessionId"`
	DurationMs      int64  `json:"durationMs"`
	Sequence        int64  `json:"sequence"`
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	VersionMismatch bool   `json:"versionMismatch"`
}

func isVersionMismatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "version mismatch") || strings.Contains(msg, "manifest version mismatch")
}
