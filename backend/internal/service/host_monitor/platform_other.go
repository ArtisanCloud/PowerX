//go:build !linux && !darwin

package hostmonitor

import "context"

func collectPlatform(_ context.Context, o *Snapshot) {
	o.Warnings = append(o.Warnings, "platform collector unsupported")
}
