package notify

import "errors"

var (
	// ErrControlChannelUnavailable 表示当前构建未启用 plugin_control，无法真实下发租户凭证。
	ErrControlChannelUnavailable = errors.New("plugin control channel unavailable")
)
