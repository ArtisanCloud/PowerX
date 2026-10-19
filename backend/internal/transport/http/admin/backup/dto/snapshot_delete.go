package dto

// SnapshotDeleteRequest 不接受路径、数据库或快照目录等调用方覆盖。
type SnapshotDeleteRequest struct {
	Confirmation    string `header:"X-Snapshot-Confirmation"`
	DeleteProtected string `header:"X-Snapshot-Delete-Protected"`
}
