package pgvector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvectorlib "github.com/pgvector/pgvector-go"
)

type store struct {
	pool      *pgxpool.Pool
	cfg       Config
	tableName string
}

// init 注册驱动。
func init() {
	vectorstore.Register(vectorstore.DriverPGVector, func(opts interface{}) (vectorstore.Store, error) {
		var cfg Config
		switch v := opts.(type) {
		case Config:
			cfg = v
		case *Config:
			if v != nil {
				cfg = *v
			}
		case nil:
			cfg = Config{}
		default:
			return nil, fmt.Errorf("pgvector: %w", vectorstore.ErrInvalidConfig)
		}
		return New(cfg)
	})
}

// New 创建 pgvector 存储实例。
func New(cfg Config) (vectorstore.Store, error) {
	cfg = cfg.WithDefaults()
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, fmt.Errorf("pgvector: %w (dsn required)", vectorstore.ErrInvalidConfig)
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, err
	}

	s := &store{
		pool:      pool,
		cfg:       cfg,
		tableName: fmt.Sprintf("%s.%s", quoteIdent(cfg.Schema), quoteIdent(cfg.Table)),
	}

	if cfg.EnableMigrations {
		if err := s.runMigrations(context.Background()); err != nil {
			pool.Close()
			return nil, err
		}
	}

	return s, nil
}

func (s *store) Driver() string {
	return vectorstore.DriverPGVector
}

func (s *store) Close(ctx context.Context) error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

func (s *store) Upsert(ctx context.Context, space uuid.UUID, vectors []vectorstore.VectorRecord) error {
	if len(vectors) == 0 {
		return nil
	}
	batchSize := s.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 128
	}
	insertSQL := fmt.Sprintf(`INSERT INTO %s (space_uuid, chunk_uuid, embedding, metadata, updated_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (space_uuid, chunk_uuid)
DO UPDATE SET embedding = EXCLUDED.embedding, metadata = EXCLUDED.metadata, updated_at = NOW()`, s.tableName)

	for start := 0; start < len(vectors); start += batchSize {
		end := start + batchSize
		if end > len(vectors) {
			end = len(vectors)
		}
		batch := pgx.Batch{}
		for _, vec := range vectors[start:end] {
			payload, _ := json.Marshal(vec.Metadata)
			batch.Queue(insertSQL,
				space,
				vec.ChunkID,
				pgvectorlib.NewVector(vec.Embedding),
				payload,
			)
		}
		if err := s.executeBatch(ctx, &batch); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) DeleteByChunkIDs(ctx context.Context, space uuid.UUID, chunkIDs []uuid.UUID) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	sql := fmt.Sprintf(`DELETE FROM %s WHERE space_uuid = $1 AND chunk_uuid = ANY($2)`, s.tableName)
	_, err := s.pool.Exec(ctx, sql, space, chunkIDs)
	return err
}

func (s *store) DropSpace(ctx context.Context, space uuid.UUID) error {
	sql := fmt.Sprintf(`DELETE FROM %s WHERE space_uuid = $1`, s.tableName)
	_, err := s.pool.Exec(ctx, sql, space)
	return err
}

func (s *store) Query(ctx context.Context, req vectorstore.QueryRequest) (vectorstore.QueryResponse, error) {
	if len(req.Embedding) == 0 {
		return vectorstore.QueryResponse{}, fmt.Errorf("pgvector: empty query vector")
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	sb := strings.Builder{}
	sb.WriteString("SELECT chunk_uuid, metadata, embedding <=> $2 AS score FROM ")
	sb.WriteString(s.tableName)
	sb.WriteString(" WHERE space_uuid = $1")

	args := []interface{}{req.SpaceID, pgvectorlib.NewVector(req.Embedding)}
	argIndex := 3
	if len(req.Filters) > 0 {
		payload, _ := json.Marshal(req.Filters)
		sb.WriteString(fmt.Sprintf(" AND metadata @> $%d", argIndex))
		args = append(args, payload)
		argIndex++
	}
	if req.MinScore > 0 {
		sb.WriteString(fmt.Sprintf(" AND embedding <=> $2 <= $%d", argIndex))
		args = append(args, req.MinScore)
		argIndex++
	}
	sb.WriteString(fmt.Sprintf(" ORDER BY embedding <=> $2 ASC LIMIT $%d", argIndex))
	args = append(args, topK)

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return vectorstore.QueryResponse{}, err
	}
	defer rows.Close()

	resp := vectorstore.QueryResponse{}
	for rows.Next() {
		var chunk uuid.UUID
		var metaRaw []byte
		var score float64
		if err := rows.Scan(&chunk, &metaRaw, &score); err != nil {
			return vectorstore.QueryResponse{}, err
		}
		match := vectorstore.QueryMatch{ChunkID: chunk, Score: score}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &match.Metadata)
		}
		resp.Matches = append(resp.Matches, match)
	}
	return resp, rows.Err()
}

func (s *store) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		return err
	}
	// 额外检查 pgvector 扩展是否可用。仅用于更早、更明确地暴露环境问题：
	// - EnableMigrations=true 时，会尝试 CREATE EXTENSION IF NOT EXISTS vector（需要权限）
	// - EnableMigrations=false 时，这里至少能提示“vector 扩展缺失”
	var ok int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM pg_extension WHERE extname = 'vector' LIMIT 1`).Scan(&ok)
	if err == nil && ok == 1 {
		return nil
	}
	// 如果未命中，Scan 会返回 pgx.ErrNoRows：这就是“缺少 vector 扩展”，应明确报错。
	if err == pgx.ErrNoRows {
		return fmt.Errorf("pgvector: extension \"vector\" not found (install pgvector or enable migrations to create it)")
	}
	// 其它查询错误（权限/兼容性等）不要误报，让 Ping 结果决定健康。
	return nil
}

func (s *store) runMigrations(ctx context.Context) error {
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
            space_uuid uuid NOT NULL,
            chunk_uuid uuid NOT NULL,
            embedding vector(%d) NOT NULL,
            metadata jsonb,
            updated_at timestamptz NOT NULL DEFAULT NOW(),
            PRIMARY KEY (space_uuid, chunk_uuid)
        )`, s.tableName, s.cfg.Dimensions),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_space_idx ON %s (space_uuid)`, sanitizeIdentifier(s.cfg.Table)+"_space_idx", s.tableName),
	}
	// optional ivfflat index
	statements = append(statements, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_embedding_idx ON %s USING ivfflat (embedding vector_l2_ops) WITH (lists = %d)`, sanitizeIdentifier(s.cfg.Table)+"_embedding_idx", s.tableName, s.cfg.Lists))
	for _, stmt := range statements {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) executeBatch(ctx context.Context, batch *pgx.Batch) error {
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < batch.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func quoteIdent(ident string) string {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return ident
	}
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(ident, "\"", ""))
}

func sanitizeIdentifier(ident string) string {
	ident = strings.TrimSpace(ident)
	ident = strings.ReplaceAll(ident, "\"", "")
	ident = strings.ReplaceAll(ident, " ", "_")
	return ident
}
