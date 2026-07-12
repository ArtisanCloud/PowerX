package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	startMockOpts = struct {
		api          string
		token        string
		pluginID     string
		environment  string
		ttl          time.Duration
		httpPort     int
		grpcPort     int
		capabilities []string
	}{
		ttl: 10 * time.Minute,
	}

	startMockCmd = &cobra.Command{
		Use:   "start",
		Short: "Register a local plugin debug host session",
		RunE:  runStartMock,
	}
)

func init() {
	Command.AddCommand(startMockCmd)
	startMockCmd.Flags().StringVar(&startMockOpts.api, "api", "http://localhost:8077/api", "PowerX Admin API base URL")
	startMockCmd.Flags().StringVar(&startMockOpts.token, "token", "", "Bearer token for host API authentication")
	startMockCmd.Flags().StringVar(&startMockOpts.pluginID, "plugin-id", "", "Plugin identifier to preload")
	startMockCmd.Flags().StringVar(&startMockOpts.environment, "environment", "local", "Host environment label")
	startMockCmd.Flags().DurationVar(&startMockOpts.ttl, "ttl", startMockOpts.ttl, "Requested host TTL (e.g., 15m)")
	startMockCmd.Flags().IntVar(&startMockOpts.httpPort, "http-port", 51701, "Local debug host HTTP port")
	startMockCmd.Flags().IntVar(&startMockOpts.grpcPort, "grpc-port", 52701, "Local debug host gRPC port")
	startMockCmd.Flags().StringSliceVar(&startMockOpts.capabilities, "capability", []string{"debug.hot_reload"}, "Capabilities to expose on the local debug host")
}

func runStartMock(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(startMockOpts.api) == "" {
		return errors.New("api base URL is required")
	}
	if strings.TrimSpace(startMockOpts.pluginID) == "" {
		return errors.New("plugin-id is required")
	}

	payload := map[string]any{
		"pluginId":     startMockOpts.pluginID,
		"environment":  startMockOpts.environment,
		"ttlSeconds":   int(startMockOpts.ttl.Seconds()),
		"httpPort":     startMockOpts.httpPort,
		"grpcPort":     startMockOpts.grpcPort,
		"capabilities": startMockOpts.capabilities,
	}

	var resp struct {
		Data map[string]any `json:"data"`
		Err  string         `json:"error"`
	}
	if err := doHostRequest(cmd.Context(), http.MethodPost, "/internal/plugins/debug-hosts", payload, &resp); err != nil {
		return err
	}
	if resp.Err != "" {
		return errors.New(resp.Err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Debug host registered: %v\n", resp.Data)
	return nil
}

func doHostRequest(ctx context.Context, method, path string, payload any, dest any) error {
	base := strings.TrimRight(startMockOpts.api, "/")
	if base == "" {
		return errors.New("invalid api base")
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
	token := strings.TrimSpace(startMockOpts.token)
	if token != "" {
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
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
		return fmt.Errorf("host request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(data, dest)
}
