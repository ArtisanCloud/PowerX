package backup_ops

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestRestoreCopyClientEndpoint(t *testing.T) {
	cfg, err := pgx.ParseConfig("host=localhost port=5432 dbname=original user=core sslmode=disable")
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		opts RestoreCopyClientOptions
		host string
		port uint16
		mode string
	}{
		{name: "systemctl keeps its source address", host: "localhost", port: 5432, mode: "direct"},
		{name: "explicit desktop endpoint", opts: RestoreCopyClientOptions{RequireExplicit: true, Host: "db.ops.example.com", Port: 25432}, host: "db.ops.example.com", port: 25432, mode: "direct"},
		{name: "ssh separates laptop and server ports", opts: RestoreCopyClientOptions{AccessMode: "ssh_tunnel", Host: "127.0.0.1", Port: 15433, TunnelHost: "127.0.0.1", TunnelPort: 25432}, host: "127.0.0.1", port: 15433, mode: "ssh_tunnel"},
		{name: "IPv6 endpoint", opts: RestoreCopyClientOptions{Host: "2001:db8::1", Port: 5433}, host: "2001:db8::1", port: 5433, mode: "direct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := resolveRestoreCopyClientEndpoint(cfg, tc.opts, "disable")
			require.NoError(t, err)
			require.Equal(t, tc.host, endpoint.Host)
			require.Equal(t, tc.port, endpoint.Port)
			require.Equal(t, tc.mode, endpoint.AccessMode)
			require.Equal(t, "disable", endpoint.SSLMode)
			if tc.mode == "ssh_tunnel" {
				require.Equal(t, &RestoreCopySSHTarget{Host: "127.0.0.1", Port: 25432}, endpoint.Tunnel)
			} else {
				require.Nil(t, endpoint.Tunnel)
			}
			// 任何客户端设置都不能改变实际恢复服务器/业务库。
			require.Equal(t, "localhost", cfg.Host)
			require.EqualValues(t, 5432, cfg.Port)
			require.Equal(t, "original", cfg.Database)
		})
	}
	endpoint, err := resolveRestoreCopyClientEndpoint(cfg, RestoreCopyClientOptions{SSLMode: "verify-full"}, "disable")
	require.NoError(t, err)
	require.Equal(t, "verify-full", endpoint.SSLMode)
	_, err = resolveRestoreCopyClientEndpoint(cfg, RestoreCopyClientOptions{RequireExplicit: true}, "disable")
	require.Equal(t, 409, dto.StatusCode(err))
	require.Equal(t, "RESTORE_CONNECTION_NOT_CONFIGURED", dto.CodeOf(err))
	socketCfg := cfg.Copy()
	socketCfg.Host = "/var/run/postgresql"
	_, err = resolveRestoreCopyClientEndpoint(socketCfg, RestoreCopyClientOptions{}, "disable")
	require.Equal(t, 409, dto.StatusCode(err))
}

func TestRestoreCopyClientRejectsInvalidConfiguration(t *testing.T) {
	for _, opts := range []RestoreCopyClientOptions{
		{AccessMode: "auto-proxy"},
		{Host: "db.example.com"},
		{Port: 5432},
		{Host: "https://db.example.com", Port: 5432},
		{Host: "postgres:5432", Port: 5432},
		{Host: "db.example.com\n", Port: 5432, SSLMode: "invalid"},
		{Host: "user@db.example.com", Port: 5432},
		{Host: "0.0.0.0", Port: 5432},
		{Host: "::", Port: 5432},
		{Host: "db1,db2", Port: 5432},
		{Host: "db.example.com", Port: 5432, TunnelPort: 5432},
		{AccessMode: "ssh_tunnel", Host: "db.example.com", Port: 15432, TunnelHost: "127.0.0.1", TunnelPort: 5432},
		{AccessMode: "ssh_tunnel", Host: "127.0.0.1", Port: 15432},
		{AccessMode: "ssh_tunnel", Host: "127.0.0.1", Port: 15432, TunnelHost: "127.0.0.1"},
	} {
		_, err := resolveRestoreCopyClientEndpoint(nil, opts, "disable")
		require.Equal(t, 503, dto.StatusCode(err), "%+v", opts)
	}
}

func TestRestoreCopyClientEnvironment(t *testing.T) {
	for _, name := range []string{"POWERX_MODE", "POWERX_OPS_RESTORE_ACCESS_MODE", "POWERX_OPS_RESTORE_CLIENT_HOST", "POWERX_OPS_RESTORE_CLIENT_PORT", "POWERX_OPS_RESTORE_CLIENT_SSL_MODE", "POWERX_OPS_RESTORE_TUNNEL_HOST", "POWERX_OPS_RESTORE_TUNNEL_PORT"} {
		t.Setenv(name, "")
	}
	t.Setenv("POWERX_MODE", "docker")
	t.Setenv("POWERX_OPS_RESTORE_CLIENT_HOST", "127.0.0.1")
	t.Setenv("POWERX_OPS_RESTORE_CLIENT_PORT", "15432")
	opts, err := restoreCopyClientOptionsFromEnv()
	require.NoError(t, err)
	require.True(t, opts.RequireExplicit)
	require.Equal(t, "127.0.0.1", opts.Host)
	require.EqualValues(t, 15432, opts.Port)
	for _, value := range []string{"0", "-1", "65536", "5432:5432", "secret-value"} {
		t.Setenv("POWERX_OPS_RESTORE_CLIENT_PORT", value)
		_, err := restoreCopyClientOptionsFromEnv()
		require.Equal(t, 503, dto.StatusCode(err))
		require.NotContains(t, err.Error(), value)
	}
}

func TestRestoreCopyMissingClientEndpointDoesNotRotateCredentials(t *testing.T) {
	store, err := newRestoreCopyStore(t.TempDir(), "source")
	require.NoError(t, err)
	id := uuid.NewString()
	token := strings.ReplaceAll(id, "-", "")
	record := copyLedger{RestoreCopy: RestoreCopy{ID: id, Database: "powerx_restore_copy_" + token, Status: "success"}, SourceKey: "source", OwnerRole: "pxrc_owner_" + token, ReaderRole: "pxrc_read_" + token, ReaderExpiresAt: time.Now().UTC().Add(time.Hour)}
	require.NoError(t, store.save(record))
	before, err := store.read(id)
	require.NoError(t, err)
	beforeJSON, err := json.Marshal(before)
	require.NoError(t, err)
	// 不提供可拨号 DB；正确顺序应在尝试连接数据库及 ALTER ROLE 前返回配置错误。
	s := &RestoreCopyService{store: store, ctx: context.Background(), sourceKey: "source", cfg: &pgx.ConnConfig{}, clientConnection: RestoreCopyClientOptions{RequireExplicit: true}}
	connection, err := s.Connection(reqctx.WithIsRoot(context.Background(), true), id)
	require.Equal(t, 409, dto.StatusCode(err))
	require.Equal(t, "RESTORE_CONNECTION_NOT_CONFIGURED", dto.CodeOf(err))
	require.Empty(t, connection.Password)
	beforeAfter, err := store.read(id)
	require.NoError(t, err)
	afterJSON, err := json.Marshal(beforeAfter)
	require.NoError(t, err)
	require.JSONEq(t, string(beforeJSON), string(afterJSON))
}
