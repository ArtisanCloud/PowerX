package eventfabric

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminOverviewHandlerOptions struct {
	DB        *gorm.DB
	Directory *directory.DirectoryService
	Enabled   bool
}

type AdminOverviewHandler struct {
	db        *gorm.DB
	directory *directory.DirectoryService
	enabled   bool
}

func NewAdminOverviewHandler(opts AdminOverviewHandlerOptions) *AdminOverviewHandler {
	return &AdminOverviewHandler{
		db:        opts.DB,
		directory: opts.Directory,
		enabled:   opts.Enabled,
	}
}

type topicDTO struct {
	ID           string `json:"id"`
	UUID         string `json:"uuid"`
	FullTopic    string `json:"full_topic"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Lifecycle    string `json:"lifecycle"`
	PayloadFormat string `json:"payload_format"`
	MaxRetry     int    `json:"max_retry"`
	AckTimeout   int    `json:"ack_timeout_sec"`
	VersionMode  string `json:"versioning_mode"`
	DeprecatedAt string `json:"deprecated_at,omitempty"`
}

type groupedCount struct {
	TopicUUID string           `json:"topic_uuid"`
	FullTopic string           `json:"full_topic,omitempty"`
	ByStatus  map[string]int64 `json:"by_status"`
	Total     int64            `json:"total"`
}

type overviewReplayTaskDTO struct {
	ID            string `json:"id"`
	TopicUUID     string `json:"topic_uuid"`
	FullTopic     string `json:"full_topic,omitempty"`
	TraceID       string `json:"trace_id,omitempty"`
	Status        string `json:"status"`
	Shadow        bool   `json:"shadow"`
	RequestedBy   string `json:"requested_by,omitempty"`
	SubmittedAt   string `json:"submitted_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	ResultCount   int    `json:"result_count"`
}

func (h *AdminOverviewHandler) GetOverview(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("event_fabric overview unavailable", nil))
		return
	}
	if !h.enabled {
		dto.RespondErrorFrom(c, dto.NewError(503, "event_fabric 未启用", nil))
		return
	}

	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid tenant uuid", err))
		return
	}

	namespace := strings.TrimSpace(c.Query("namespace"))
	name := strings.TrimSpace(c.Query("name"))
	subscriberID := strings.TrimSpace(c.Query("subscriber_id"))
	if subscriberID == "" {
		subscriberID = "core.knowledge_space.reprocess"
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	topics, topicMap, err := h.listTopics(c, tenantUUID, namespace, name)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list topics failed", err))
		return
	}

	dlqStats, err := h.queryDLQCounts(c, tenantUUID, topicMap)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query dlq stats failed", err))
		return
	}

	attemptStats, err := h.queryAttemptCounts(c, tenantUUID, subscriberID, topicMap)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query delivery attempt stats failed", err))
		return
	}

	replayTasks, err := h.queryReplayTasks(c, tenantUUID, topicMap, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query replay tasks failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"now":         time.Now().UTC().Format(time.RFC3339),
		"tenant_uuid": tenantUUID,
		"filters": gin.H{
			"namespace":      namespace,
			"name":           name,
			"subscriber_id":  subscriberID,
			"replay_task_max": limit,
		},
		"topics": topics,
		"stats": gin.H{
			"dlq": gin.H{
				"by_topic": dlqStats,
				"total":    sumGrouped(dlqStats),
			},
			"delivery_attempts": gin.H{
				"subscriber_id": subscriberID,
				"by_topic":      attemptStats,
				"total":         sumGrouped(attemptStats),
			},
			"replay_tasks": gin.H{
				"recent": replayTasks,
			},
		},
	})
}

func (h *AdminOverviewHandler) listTopics(c *gin.Context, tenantUUID, namespace, name string) ([]topicDTO, map[string]string, error) {
	ctx := c.Request.Context()

	typeRow := func(id, fullTopic, ns, nm, lifecycle, payloadFormat string, maxRetry, ackTimeout int, versionMode string, deprecatedAt *time.Time) topicDTO {
		out := topicDTO{
			ID:            id,
			UUID:          id,
			FullTopic:     fullTopic,
			Namespace:     ns,
			Name:          nm,
			Lifecycle:     lifecycle,
			PayloadFormat: payloadFormat,
			MaxRetry:      maxRetry,
			AckTimeout:    ackTimeout,
			VersionMode:   versionMode,
		}
		if deprecatedAt != nil {
			out.DeprecatedAt = deprecatedAt.UTC().Format(time.RFC3339)
		}
		return out
	}

	if h.directory != nil {
		list, _, err := h.directory.ListTopics(ctx, repositoryQueryForTopics(tenantUUID, namespace))
		if err != nil {
			return nil, nil, err
		}
		filtered := make([]topicDTO, 0, len(list))
		topicMap := make(map[string]string, len(list))
		for _, t := range list {
			if t == nil {
				continue
			}
			if name != "" && !strings.EqualFold(strings.TrimSpace(t.Name), name) {
				continue
			}
			id := strings.TrimSpace(t.ID)
			if id == "" {
				continue
			}
			row := typeRow(
				id,
				strings.TrimSpace(t.FullTopic),
				strings.TrimSpace(t.Namespace),
				strings.TrimSpace(t.Name),
				string(t.Lifecycle),
				strings.TrimSpace(t.PayloadFormat),
				int(t.MaxRetry),
				int(t.AckTimeoutSec),
				strings.TrimSpace(t.VersioningMode),
				t.DeprecatedAt,
			)
			filtered = append(filtered, row)
			topicMap[id] = row.FullTopic
		}
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].FullTopic < filtered[j].FullTopic })
		return filtered, topicMap, nil
	}

	var items []*eventfabricmodel.TopicDefinition
	query := h.db.WithContext(ctx).Model(&eventfabricmodel.TopicDefinition{}).Where("tenant_key = ?", tenantUUID)
	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	if err := query.Order("created_at DESC").Limit(200).Find(&items).Error; err != nil {
		return nil, nil, err
	}

	filtered := make([]topicDTO, 0, len(items))
	topicMap := make(map[string]string, len(items))
	for _, t := range items {
		if t == nil {
			continue
		}
		if name != "" && !strings.EqualFold(strings.TrimSpace(t.Name), name) {
			continue
		}
		id := t.UUID.String()
		row := typeRow(
			id,
			strings.TrimSpace(t.FullTopic),
			strings.TrimSpace(t.Namespace),
			strings.TrimSpace(t.Name),
			string(t.Lifecycle),
			strings.TrimSpace(t.PayloadFormat),
			t.MaxRetry,
			t.AckTimeoutSec,
			strings.TrimSpace(t.VersioningMode),
			t.DeprecatedAt,
		)
		filtered = append(filtered, row)
		topicMap[id] = row.FullTopic
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].FullTopic < filtered[j].FullTopic })
	return filtered, topicMap, nil
}

func repositoryQueryForTopics(tenantUUID, namespace string) eventfabrepo.QueryContext {
	return eventfabrepo.QueryContext{
		Filter: eventfabrepo.TopicFilter{
			TenantID:  tenantUUID,
			Namespace: namespace,
		},
		Page: eventfabrepo.PageOptions{
			Limit:  200,
			Offset: 0,
		},
		Sort: eventfabrepo.SortOption{
			Field: "created_at",
			Desc:  true,
		},
	}
}

type topicStatusCountRow struct {
	TopicUUID uuid.UUID
	Status    string
	Count     int64
}

func (h *AdminOverviewHandler) queryDLQCounts(c *gin.Context, tenantUUID string, topicMap map[string]string) ([]groupedCount, error) {
	if len(topicMap) == 0 {
		return []groupedCount{}, nil
	}
	ctx := c.Request.Context()

	topicUUIDs := keysUUID(topicMap)

	var rows []topicStatusCountRow
	if err := h.db.WithContext(ctx).
		Model(&eventfabricmodel.DlqMessage{}).
		Select("topic_uuid as topic_uuid, status, count(*) as count").
		Where("tenant_key = ? AND topic_uuid IN ?", tenantUUID, topicUUIDs).
		Group("topic_uuid, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	return toGrouped(rows, topicMap), nil
}

func (h *AdminOverviewHandler) queryAttemptCounts(c *gin.Context, tenantUUID, subscriberID string, topicMap map[string]string) ([]groupedCount, error) {
	if subscriberID == "" || len(topicMap) == 0 {
		return []groupedCount{}, nil
	}
	ctx := c.Request.Context()

	topicUUIDs := keysUUID(topicMap)
	attemptTable := (&eventfabricmodel.DeliveryAttempt{}).TableName()
	envelopeTable := (&eventfabricmodel.EventEnvelope{}).TableName()

	var rows []topicStatusCountRow
	if err := h.db.WithContext(ctx).
		Table(attemptTable+" as a").
		Select("e.topic_uuid as topic_uuid, a.status, count(*) as count").
		Joins("JOIN " + envelopeTable + " as e ON e.uuid = a.envelope_uuid").
		Where("a.tenant_key = ? AND a.subscriber_id = ? AND e.topic_uuid IN ?", tenantUUID, subscriberID, topicUUIDs).
		Group("e.topic_uuid, a.status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return toGrouped(rows, topicMap), nil
}

func (h *AdminOverviewHandler) queryReplayTasks(c *gin.Context, tenantUUID string, topicMap map[string]string, limit int) ([]overviewReplayTaskDTO, error) {
	ctx := c.Request.Context()

	replayTable := (&eventfabricmodel.ReplayRequest{}).TableName()
	topicTable := (&eventfabricmodel.TopicDefinition{}).TableName()

	query := h.db.WithContext(ctx).
		Table(replayTable+" as r").
		Select(strings.Join([]string{
			"r.uuid as id",
			"r.topic_uuid as topic_uuid",
			"t.full_topic as full_topic",
			"r.trace_id as trace_id",
			"r.status as status",
			"r.shadow as shadow",
			"r.issued_by as requested_by",
			"r.submitted_at as submitted_at",
			"r.completed_at as completed_at",
			"r.failure_reason as failure_reason",
			"r.result_count as result_count",
		}, ", ")).
		Joins("LEFT JOIN " + topicTable + " as t ON t.uuid = r.topic_uuid").
		Where("r.tenant_key = ?", tenantUUID).
		Order("r.submitted_at DESC").
		Limit(limit)

	if len(topicMap) > 0 {
		query = query.Where("r.topic_uuid IN ?", keysUUID(topicMap))
	}

	type row struct {
		ID            string
		TopicUUID     uuid.UUID
		FullTopic     string
		TraceID       string
		Status        string
		Shadow        bool
		RequestedBy   string
		SubmittedAt   time.Time
		CompletedAt   *time.Time
		FailureReason string
		ResultCount   int
	}
	var rows []row
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]overviewReplayTaskDTO, 0, len(rows))
	for _, r := range rows {
		topicID := r.TopicUUID.String()
		item := overviewReplayTaskDTO{
			ID:            r.ID,
			TopicUUID:     topicID,
			FullTopic:     strings.TrimSpace(r.FullTopic),
			TraceID:       strings.TrimSpace(r.TraceID),
			Status:        r.Status,
			Shadow:        r.Shadow,
			RequestedBy:   strings.TrimSpace(r.RequestedBy),
			SubmittedAt:   r.SubmittedAt.UTC().Format(time.RFC3339),
			FailureReason: strings.TrimSpace(r.FailureReason),
			ResultCount:   r.ResultCount,
		}
		if item.FullTopic == "" {
			item.FullTopic = topicMap[topicID]
		}
		if r.CompletedAt != nil {
			item.CompletedAt = r.CompletedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysUUID(m map[string]string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for k := range m {
		if id, err := uuid.Parse(strings.TrimSpace(k)); err == nil {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func toGrouped(rows []topicStatusCountRow, topicMap map[string]string) []groupedCount {
	by := map[string]*groupedCount{}
	for _, r := range rows {
		id := r.TopicUUID.String()
		if id == uuid.Nil.String() {
			continue
		}
		if id == "" {
			continue
		}
		g, ok := by[id]
		if !ok {
			g = &groupedCount{
				TopicUUID: id,
				FullTopic: topicMap[id],
				ByStatus:  map[string]int64{},
			}
			by[id] = g
		}
		st := strings.ToLower(strings.TrimSpace(r.Status))
		if st == "" {
			st = "unknown"
		}
		g.ByStatus[st] += r.Count
		g.Total += r.Count
	}

	out := make([]groupedCount, 0, len(by))
	for _, v := range by {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullTopic < out[j].FullTopic })
	return out
}

func sumGrouped(items []groupedCount) int64 {
	var total int64
	for _, it := range items {
		total += it.Total
	}
	return total
}
