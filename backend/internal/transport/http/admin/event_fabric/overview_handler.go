package eventfabric

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type AdminOverviewHandlerOptions struct {
	DB        *gorm.DB
	Directory *directory.DirectoryService
	Redis     *redis.Client
	Enabled   bool
}

type AdminOverviewHandler struct {
	db        *gorm.DB
	directory *directory.DirectoryService
	redis     *redis.Client
	history   *eventfabrepo.TaskHistoryRepository
	enabled   bool
}

func NewAdminOverviewHandler(opts AdminOverviewHandlerOptions) *AdminOverviewHandler {
	return &AdminOverviewHandler{
		db:        opts.DB,
		directory: opts.Directory,
		redis:     opts.Redis,
		history: func() *eventfabrepo.TaskHistoryRepository {
			if opts.DB == nil {
				return nil
			}
			return eventfabrepo.NewTaskHistoryRepository(opts.DB)
		}(),
		enabled: opts.Enabled,
	}
}

type taskQueueSubscriberStats struct {
	SubscriberID string `json:"subscriber_id"`
	TenantKey    string `json:"tenant_key"`
	Pending      int64  `json:"pending"`
	Deferred     int64  `json:"deferred"`
	Processing   int64  `json:"processing"`
	Inflight     int64  `json:"inflight"`
	TotalTasks   int64  `json:"total_tasks"`
}

type taskQueueMessageDTO struct {
	ID       string            `json:"id"`
	Topic    string            `json:"topic"`
	TraceID  string            `json:"trace_id,omitempty"`
	Attempt  int               `json:"attempt"`
	Visible  string            `json:"visible_at,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type taskHistoryDTO struct {
	TaskID      string `json:"task_id"`
	TenantKey   string `json:"tenant_key"`
	Subscriber  string `json:"subscriber_id"`
	Topic       string `json:"topic"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	TraceID     string `json:"trace_id,omitempty"`
	Attempt     int    `json:"attempt"`
	Source      string `json:"source"`
	SubmittedAt string `json:"submitted_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	LastSeenAt  string `json:"last_seen_at,omitempty"`
}

type topicDTO struct {
	ID            string `json:"id"`
	UUID          string `json:"uuid"`
	FullTopic     string `json:"full_topic"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	Lifecycle     string `json:"lifecycle"`
	PayloadFormat string `json:"payload_format"`
	MaxRetry      int    `json:"max_retry"`
	AckTimeout    int    `json:"ack_timeout_sec"`
	VersionMode   string `json:"versioning_mode"`
	DeprecatedAt  string `json:"deprecated_at,omitempty"`
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
		subscriberID = eventbus.SubscriberKnowledgeSpaceReprocess
	}
	subscriberID = canonicalSubscriberID(subscriberID)
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

	taskQueueStats, err := h.queryTaskQueueStats(c, tenantUUID, subscriberID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query task queue stats failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"now":         time.Now().UTC().Format(time.RFC3339),
		"tenant_uuid": tenantUUID,
		"filters": gin.H{
			"namespace":       namespace,
			"name":            name,
			"subscriber_id":   subscriberID,
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
			"task_queue": taskQueueStats,
		},
	})
}

func (h *AdminOverviewHandler) GetTaskQueueStats(c *gin.Context) {
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

	subscriberID := strings.TrimSpace(c.Query("subscriber_id"))
	if subscriberID == "" {
		subscriberID = eventbus.SubscriberKnowledgeSpaceReprocess
	}
	subscriberID = canonicalSubscriberID(subscriberID)

	taskQueueStats, err := h.queryTaskQueueStats(c, tenantUUID, subscriberID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("query task queue stats failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"now":           time.Now().UTC().Format(time.RFC3339),
		"tenant_uuid":   tenantUUID,
		"subscriber_id": subscriberID,
		"task_queue":    taskQueueStats,
	})
}

func (h *AdminOverviewHandler) GetTaskQueueMessages(c *gin.Context) {
	if h == nil || h.db == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("event_fabric overview unavailable", nil))
		return
	}
	if !h.enabled {
		dto.RespondErrorFrom(c, dto.NewError(503, "event_fabric 未启用", nil))
		return
	}
	if h.redis == nil {
		dto.ResponseSuccess(c, gin.H{
			"now":      time.Now().UTC().Format(time.RFC3339),
			"messages": gin.H{"pending": []taskQueueMessageDTO{}, "deferred": []taskQueueMessageDTO{}, "processing": []taskQueueMessageDTO{}, "inflight": []taskQueueMessageDTO{}},
		})
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

	tenantKey := strings.TrimSpace(c.Query("tenant_key"))
	if tenantKey == "" {
		tenantKey = tenantUUID
	}
	subscriberID := canonicalSubscriberID(strings.TrimSpace(c.Query("subscriber_id")))
	if subscriberID == "" {
		subscriberID = eventbus.SubscriberKnowledgeSpaceReprocess
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	pending, err := h.readTaskQueueState(c, "q", tenantKey, subscriberID, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("read pending queue failed", err))
		return
	}
	deferred, err := h.readTaskQueueState(c, "d", tenantKey, subscriberID, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("read deferred queue failed", err))
		return
	}
	processing, err := h.readTaskQueueState(c, "p", tenantKey, subscriberID, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("read processing queue failed", err))
		return
	}
	inflight, err := h.readTaskQueueState(c, "i", tenantKey, subscriberID, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("read inflight queue failed", err))
		return
	}
	history, err := h.queryTaskHistory(c, tenantKey, subscriberID, limit)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("read task history failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"now":           time.Now().UTC().Format(time.RFC3339),
		"tenant_uuid":   tenantUUID,
		"tenant_key":    tenantKey,
		"subscriber_id": subscriberID,
		"limit":         limit,
		"messages": gin.H{
			"pending":    pending,
			"deferred":   deferred,
			"processing": processing,
			"inflight":   inflight,
		},
		"history": history,
	})
}

func (h *AdminOverviewHandler) queryTaskQueueStats(c *gin.Context, tenantUUID, subscriberID string) (gin.H, error) {
	if h.redis == nil {
		return gin.H{
			"pending":       int64(0),
			"deferred":      int64(0),
			"processing":    int64(0),
			"inflight":      int64(0),
			"by_subscriber": []taskQueueSubscriberStats{},
		}, nil
	}

	prefix := "event_fabric:task"
	ordered := []taskQueueSubscriberStats{}
	type pair struct {
		TenantKey    string
		SubscriberID string
	}
	pairs := make([]pair, 0, 32)

	appendStats := func(tenantKey, subscriber string) error {
		tenantKey = strings.TrimSpace(tenantKey)
		subscriber = canonicalSubscriberID(subscriber)
		if tenantKey == "" || subscriber == "" {
			return nil
		}
		qKey := prefix + ":q:" + tenantKey + ":" + subscriber
		dKey := prefix + ":d:" + tenantKey + ":" + subscriber
		pKey := prefix + ":p:" + tenantKey + ":" + subscriber
		iKey := prefix + ":i:" + tenantKey + ":" + subscriber

		pending, err := h.redis.LLen(c.Request.Context(), qKey).Result()
		if err != nil {
			return err
		}
		deferred, err := h.redis.ZCard(c.Request.Context(), dKey).Result()
		if err != nil {
			return err
		}
		processing, err := h.redis.LLen(c.Request.Context(), pKey).Result()
		if err != nil {
			return err
		}
		inflight, err := h.redis.HLen(c.Request.Context(), iKey).Result()
		if err != nil {
			return err
		}

		ordered = append(ordered, taskQueueSubscriberStats{
			SubscriberID: subscriber,
			TenantKey:    tenantKey,
			Pending:      pending,
			Deferred:     deferred,
			Processing:   processing,
			Inflight:     inflight,
			TotalTasks:   pending + deferred + processing + inflight,
		})
		return nil
	}

	seen := map[string]struct{}{}
	addSubscriber := func(tenantKey, subscriber string) {
		key := strings.TrimSpace(tenantKey) + "::" + strings.TrimSpace(subscriber)
		if key == "::" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		pairs = append(pairs, pair{
			TenantKey:    strings.TrimSpace(tenantKey),
			SubscriberID: canonicalSubscriberID(strings.TrimSpace(subscriber)),
		})
	}

	// 基础订阅者（保障无任务时也可见）
	addSubscriber(tenantUUID, subscriberID)
	addSubscriber(tenantUUID, eventbus.SubscriberEventFabricReplay)
	addSubscriber(tenantUUID, eventbus.SubscriberKnowledgeSpaceCorpusCheck)
	addSubscriber("global", eventbus.SubscriberSystemNotificationDispatch)
	addSubscriber("global", eventbus.SubscriberAuthorizationChallengeTime)

	// 从 Redis 实时队列发现全部 tenant+subscriber 分片
	states := []string{"q", "d", "p", "i"}
	for _, state := range states {
		pattern := fmt.Sprintf("%s:%s:*:*", prefix, state)
		var cursor uint64
		for {
			keys, next, err := h.redis.Scan(c.Request.Context(), cursor, pattern, 200).Result()
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				parts := strings.Split(key, ":")
				if len(parts) < 5 {
					continue
				}
				tenantKey := strings.TrimSpace(parts[3])
				subscriber := strings.TrimSpace(strings.Join(parts[4:], ":"))
				if tenantKey == "" || subscriber == "" {
					continue
				}
				if subscriberID != "" && !strings.EqualFold(subscriber, canonicalSubscriberID(subscriberID)) {
					continue
				}
				addSubscriber(tenantKey, subscriber)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}

	// 从历史账本补齐“已消费完成但运行态为空”的分片
	if h.db != nil {
		type historyPair struct {
			TenantKey    string `gorm:"column:tenant_key"`
			SubscriberID string `gorm:"column:subscriber_id"`
		}
		rows := make([]historyPair, 0, 32)
		query := h.db.Model(&eventfabricmodel.TaskHistory{}).Select("tenant_key, subscriber_id").Distinct()
		if subscriberID != "" {
			query = query.Where("subscriber_id = ?", canonicalSubscriberID(subscriberID))
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			addSubscriber(row.TenantKey, row.SubscriberID)
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].TenantKey == pairs[j].TenantKey {
			return pairs[i].SubscriberID < pairs[j].SubscriberID
		}
		if pairs[i].TenantKey == "global" {
			return false
		}
		if pairs[j].TenantKey == "global" {
			return true
		}
		return pairs[i].TenantKey < pairs[j].TenantKey
	})
	for _, item := range pairs {
		if err := appendStats(item.TenantKey, item.SubscriberID); err != nil {
			return nil, err
		}
	}

	for idx := range ordered {
		var historyCount int64
		if err := h.db.Model(&eventfabricmodel.TaskHistory{}).
			Where("tenant_key = ? AND subscriber_id = ?", ordered[idx].TenantKey, ordered[idx].SubscriberID).
			Count(&historyCount).Error; err != nil {
			return nil, err
		}
		if historyCount > ordered[idx].TotalTasks {
			ordered[idx].TotalTasks = historyCount
		}
	}

	var pending, deferred, processing, inflight int64
	for _, item := range ordered {
		pending += item.Pending
		deferred += item.Deferred
		processing += item.Processing
		inflight += item.Inflight
	}

	return gin.H{
		"pending":       pending,
		"deferred":      deferred,
		"processing":    processing,
		"inflight":      inflight,
		"by_subscriber": ordered,
	}, nil
}

func canonicalSubscriberID(subscriber string) string {
	switch strings.TrimSpace(subscriber) {
	case "core.knowledge_space.reprocess":
		return eventbus.SubscriberKnowledgeSpaceReprocess
	case "core.knowledge_space.corpus_check":
		return eventbus.SubscriberKnowledgeSpaceCorpusCheck
	case "core.authorization.challenge_timeout":
		return eventbus.SubscriberAuthorizationChallengeTime
	case "core.system.notification_dispatch":
		return eventbus.SubscriberSystemNotificationDispatch
	default:
		return strings.TrimSpace(subscriber)
	}
}

func (h *AdminOverviewHandler) readTaskQueueState(c *gin.Context, state, tenantKey, subscriberID string, limit int) ([]taskQueueMessageDTO, error) {
	if h == nil || h.redis == nil {
		return []taskQueueMessageDTO{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	key := "event_fabric:task:" + state + ":" + strings.TrimSpace(tenantKey) + ":" + strings.TrimSpace(subscriberID)
	ctx := c.Request.Context()

	var raws []string
	switch state {
	case "q", "p":
		items, err := h.redis.LRange(ctx, key, 0, int64(limit-1)).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		raws = items
	case "d":
		items, err := h.redis.ZRange(ctx, key, 0, int64(limit-1)).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		raws = items
	case "i":
		items, err := h.redis.HVals(ctx, key).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		if len(items) > limit {
			items = items[:limit]
		}
		raws = items
	default:
		return []taskQueueMessageDTO{}, nil
	}

	out := make([]taskQueueMessageDTO, 0, len(raws))
	for _, raw := range raws {
		var msg event_bus.TaskMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		row := taskQueueMessageDTO{
			ID:       strings.TrimSpace(msg.ID),
			Topic:    strings.TrimSpace(msg.Topic),
			TraceID:  strings.TrimSpace(msg.TraceID),
			Attempt:  msg.Attempt,
			Metadata: msg.Metadata,
		}
		if !msg.VisibleAt.IsZero() {
			row.Visible = msg.VisibleAt.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	return out, nil
}

func (h *AdminOverviewHandler) queryTaskHistory(c *gin.Context, tenantKey, subscriberID string, limit int) ([]taskHistoryDTO, error) {
	if h == nil || h.history == nil {
		return []taskHistoryDTO{}, nil
	}
	records, err := h.history.ListRecent(c.Request.Context(), strings.TrimSpace(tenantKey), strings.TrimSpace(subscriberID), limit)
	if err != nil {
		return nil, err
	}
	out := make([]taskHistoryDTO, 0, len(records))
	for _, item := range records {
		if item == nil {
			continue
		}
		out = append(out, taskHistoryDTO{
			TaskID:      strings.TrimSpace(item.TaskID),
			TenantKey:   strings.TrimSpace(item.TenantKey),
			Subscriber:  strings.TrimSpace(item.SubscriberID),
			Topic:       strings.TrimSpace(item.Topic),
			Kind:        strings.TrimSpace(item.Kind),
			Status:      strings.TrimSpace(item.Status),
			TraceID:     strings.TrimSpace(item.TraceID),
			Attempt:     item.Attempt,
			Source:      strings.TrimSpace(item.Source),
			SubmittedAt: toRFC3339Ptr(item.SubmittedAt),
			CompletedAt: toRFC3339Ptr(item.CompletedAt),
			LastSeenAt:  item.LastSeenAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func toRFC3339Ptr(ts *time.Time) string {
	if ts == nil || ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
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
			TenantID:      tenantUUID,
			Namespace:     namespace,
			IncludeShared: true,
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
		Joins("JOIN "+envelopeTable+" as e ON e.uuid = a.envelope_uuid").
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
		Joins("LEFT JOIN "+topicTable+" as t ON t.uuid = r.topic_uuid").
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
