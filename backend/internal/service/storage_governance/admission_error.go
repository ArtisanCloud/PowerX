package storagegov

import (
	"errors"
	"regexp"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/jackc/pgx/v5/pgconn"
)

var admissionCauseCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,95}$`)

// admissionUnavailable 只保留安全机器码，SQL、连接信息和原始错误不进入插件响应。
func admissionUnavailable(err error) error {
	code := "STORAGE_GOVERNANCE_UNAVAILABLE"
	details := map[string]interface{}{}
	var schema *database.StorageGovernanceSchemaError
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &schema):
		code = schema.ReasonCode()
	case errors.As(err, &pg) && (pg.Code == "42P01" || pg.Code == "42703"):
		code = "STORAGE_CONTROL_SCHEMA_NOT_READY"
	case dto.CodeOf(err) == "STORAGE_CONTROL_SCHEMA_NOT_READY":
		code = "STORAGE_CONTROL_SCHEMA_NOT_READY"
	default:
		cause := dto.CodeOf(err)
		if admissionCauseCode.MatchString(cause) && (strings.HasPrefix(cause, "STORAGE_CONTROL_") || strings.HasPrefix(cause, "DATABASE_RETENTION_")) {
			details["cause_code"] = cause
		}
	}
	out := dto.NewErrorWithCode(503, code, code, nil)
	out.Details = details
	return out
}
