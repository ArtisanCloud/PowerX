package integration_gateway

import "errors"

var (
	// ErrRouteNotFound 当路由不存在时返回。
	ErrRouteNotFound = errors.New("integration gateway route not found")
	// ErrVersionConflict 当版本号与数据库不一致时返回。
	ErrVersionConflict = errors.New("integration gateway route version conflict")
	// ErrSlugConflict 当租户下路由别名重复时返回。
	ErrSlugConflict = errors.New("integration gateway route slug conflict")
)
