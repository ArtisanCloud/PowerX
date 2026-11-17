package hostmonitor

import "time"

type Capacity struct {
	Status    string  `json:"status"`
	Total     *uint64 `json:"total_bytes"`
	Available *uint64 `json:"available_bytes"`
	Used      *uint64 `json:"used_bytes"`
}
type CPU struct {
	Status       string   `json:"status"`
	LogicalCores int      `json:"logical_cores"`
	Usage        *float64 `json:"usage_percent"`
	SampleMS     int64    `json:"sample_ms"`
}
type Disk struct {
	ReadOnly *bool `json:"read_only,omitempty"`
	Capacity
	MountPoint string `json:"mount_point"`
	Filesystem string `json:"filesystem"`
}
type Interface struct {
	Up           bool     `json:"is_up"`
	Loopback     bool     `json:"is_loopback"`
	PointToPoint bool     `json:"point_to_point"`
	Name         string   `json:"name"`
	Addresses    []string `json:"addresses"`
	RX           *uint64  `json:"rx_bytes"`
	TX           *uint64  `json:"tx_bytes"`
}
type Network struct {
	Status     string      `json:"status"`
	Interfaces []Interface `json:"interfaces"`
}
type GPUDevice struct {
	Index       int      `json:"index"`
	Name        string   `json:"name"`
	Utilization *float64 `json:"utilization_percent"`
	Total       *uint64  `json:"memory_total_bytes"`
	Used        *uint64  `json:"memory_used_bytes"`
	Temperature *float64 `json:"temperature_celsius"`
}
type GPU struct {
	Status  string      `json:"status"`
	Devices []GPUDevice `json:"devices"`
}
type Snapshot struct {
	Hostname          string    `json:"hostname"`
	UptimeSeconds     *float64  `json:"uptime_seconds"`
	CollectedAt       time.Time `json:"collected_at"`
	DurationMS        int64     `json:"duration_ms"`
	OS                string    `json:"os"`
	Arch              string    `json:"arch"`
	Scope             string    `json:"scope"`
	ContainerDetected bool      `json:"container_detected"`
	ScopeNote         string    `json:"scope_note"`
	CPU               CPU       `json:"cpu"`
	Memory            Capacity  `json:"memory"`
	Swap              Capacity  `json:"swap"`
	Disks             []Disk    `json:"disks"`
	Network           Network   `json:"network"`
	GPU               GPU       `json:"gpu"`
	Warnings          []string  `json:"warnings"`
}

func ptr[T any](v T) *T { return &v }
func capacity(total, available uint64) Capacity {
	if available > total {
		available = total
	}
	return Capacity{"ok", ptr(total), ptr(available), ptr(total - available)}
}
