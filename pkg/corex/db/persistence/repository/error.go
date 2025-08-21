package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// IsUniqueViolation 判断是否为“唯一键冲突”(GORM友好版)
// 可选 constraintName 进一步精确匹配具体唯一约束（比如 uk_member_tenant_user）
func IsUniqueViolation(err error, constraintName ...string) bool {
	if err == nil {
		return false
	}
	// 1) 先看是否命中 GORM 的重复键错误
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		if len(constraintName) == 0 || constraintName[0] == "" {
			return true
		}
		// GORM 会把底层驱动的报错文本带上来，Postgres 会包含约束名：
		//  duplicate key value violates unique constraint "uk_member_tenant_user"
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, strings.ToLower(constraintName[0]))
	}

	// 2) 兜底：直接通过错误文本关键词判断（适配不同驱动/层级包裹）
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique violation") {
		if len(constraintName) == 0 || constraintName[0] == "" {
			return true
		}
		return strings.Contains(msg, strings.ToLower(constraintName[0]))
	}

	return false
}
