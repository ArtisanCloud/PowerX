package eventfabric

import (
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

// PageOptions 统一分页参数，便于仓储实现复用。
type PageOptions struct {
	Limit  int
	Offset int
}

// TopicFilter 描述主题目录的筛选条件。
type TopicFilter struct {
	TenantID   string
	Namespace  string
	Lifecycle  []model.TopicLifecycle
	IncludeDLQ bool
}

// SortOption 描述排序字段。
type SortOption struct {
	Field string
	Desc  bool
}

// QueryContext 汇总分页、过滤与排序配置，后续仓储可直接接收该结构。
type QueryContext struct {
	Filter TopicFilter
	Page   PageOptions
	Sort   SortOption
}
