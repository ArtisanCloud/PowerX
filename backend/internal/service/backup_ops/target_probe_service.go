package backup_ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type TargetProbeRequest struct {
	Driver            string
	Host              string
	Port              int
	Database          string
	Username          string
	Password          string
	SSLMode           string
	ConnectTimeoutSec int
}

type TargetProbeResult struct {
	Driver     string
	Reachable  bool
	LatencyMs  int64
	ServerInfo string
	Message    string
}

func ProbeTargetConnection(ctx context.Context, req TargetProbeRequest) (*TargetProbeResult, error) {
	driver := strings.TrimSpace(strings.ToLower(req.Driver))
	if driver == "" {
		driver = "postgres"
	}
	if driver != "postgres" {
		return nil, fmt.Errorf("%w: unsupported driver %s", ErrInvalidBackupTarget, driver)
	}

	host := strings.TrimSpace(req.Host)
	database := strings.TrimSpace(req.Database)
	username := strings.TrimSpace(req.Username)
	if host == "" || database == "" || username == "" {
		return nil, fmt.Errorf("%w: host/database/username is required", ErrInvalidBackupTarget)
	}

	port := req.Port
	if port <= 0 {
		port = 5432
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("%w: invalid port", ErrInvalidBackupTarget)
	}

	sslMode := strings.TrimSpace(req.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}

	timeoutSec := req.ConnectTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 5
	}
	if timeoutSec > 15 {
		timeoutSec = 15
	}

	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		host,
		port,
		username,
		req.Password,
		database,
		sslMode,
		timeoutSec,
	)

	start := time.Now()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBackupTargetConnectFailed, err)
	}
	defer db.Close()

	if err := db.PingContext(probeCtx); err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: timeout", ErrBackupTargetConnectFailed)
		}
		return nil, fmt.Errorf("%w: %v", ErrBackupTargetConnectFailed, err)
	}

	var serverVersion string
	if err := db.QueryRowContext(probeCtx, "SELECT version()").Scan(&serverVersion); err != nil {
		serverVersion = ""
	}

	return &TargetProbeResult{
		Driver:     driver,
		Reachable:  true,
		LatencyMs:  time.Since(start).Milliseconds(),
		ServerInfo: serverVersion,
		Message:    "连接成功",
	}, nil
}
