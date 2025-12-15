package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

// ExportFormat 定义导出格式。
type ExportFormat string

const (
	// ExportFormatCSV 生成 CSV 结果并返回下载链接。
	ExportFormatCSV ExportFormat = "csv"
	// ExportFormatJSON 直接返回 JSON 数据。
	ExportFormatJSON ExportFormat = "json"
)

// ExportFilter 描述导出的筛选条件。
type ExportFilter struct {
	TenantUUID         string
	DefinitionUUID     *uuid.UUID
	State              string
	CreatedFrom        *time.Time
	CreatedTo          *time.Time
	IncludeStepDetails bool
	Format             ExportFormat
	PageSize           int
}

// ExportStep 提供导出中的步骤明细。
type ExportStep struct {
	StepID           string
	Type             string
	State            string
	SubjectType      string
	SubjectID        string
	Attempts         int
	ToolGrantVersion int64
	LastTransitionAt time.Time
	LastError        string
}

// ExportRow 描述单个实例的导出数据。
type ExportRow struct {
	InstanceID        string
	DefinitionID      string
	DefinitionVersion int32
	State             string
	StartedAt         *time.Time
	CompletedAt       *time.Time
	TenantUUID        string
	CorrelationID     string
	Steps             []ExportStep
}

// ExportResult 返回导出执行后的结果集合。
type ExportResult struct {
	Rows        []ExportRow
	DownloadURL string
	Format      ExportFormat
	GeneratedAt time.Time
}

// ExportInstances 汇总实例与步骤信息，返回审计导出结果。
func (s *Service) ExportInstances(ctx context.Context, filter ExportFilter) (ExportResult, error) {
	if s == nil {
		return ExportResult{}, errors.New("workflow service unavailable")
	}
	tenantUUID, err := normalizeTenantUUID(filter.TenantUUID)
	if err != nil {
		return ExportResult{}, err
	}
	filter.TenantUUID = tenantUUID

	format := normalizeExportFormat(filter.Format)
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 200
	}
	if pageSize > 2000 {
		pageSize = 2000
	}

	instFilter := workflowrepo.InstanceListFilter{
		TenantUUID: tenantUUID,
		PageSize:   pageSize,
		Page:       1,
	}
	if filter.DefinitionUUID != nil {
		instFilter.DefinitionUUID = *filter.DefinitionUUID
	}
	if filter.State != "" {
		instFilter.State = strings.ToLower(filter.State)
	}
	if filter.CreatedFrom != nil {
		instFilter.From = filter.CreatedFrom
	}
	if filter.CreatedTo != nil {
		instFilter.To = filter.CreatedTo
	}

	instances, _, err := s.instances.ListInstances(ctx, instFilter)
	if err != nil {
		return ExportResult{}, err
	}

	rows := make([]ExportRow, 0, len(instances))
	includeSteps := filter.IncludeStepDetails

	for idx := range instances {
		inst := instances[idx]
		row := ExportRow{
			InstanceID:        inst.UUID.String(),
			DefinitionID:      inst.DefinitionUUID.String(),
			DefinitionVersion: inst.DefinitionVersion,
			State:             inst.State,
			TenantUUID:        inst.TenantUUID,
			CorrelationID:     inst.CorrelationID,
		}
		if inst.StartedAt != nil {
			row.StartedAt = inst.StartedAt
		}
		if inst.CompletedAt != nil {
			row.CompletedAt = inst.CompletedAt
		}

		if includeSteps {
			records, err := s.steps.ListByInstance(ctx, inst.UUID)
			if err != nil {
				return ExportResult{}, err
			}
			row.Steps = buildExportSteps(records)
		}

		rows = append(rows, row)
	}

	result := ExportResult{
		Rows:        rows,
		DownloadURL: "",
		Format:      format,
		GeneratedAt: s.now().UTC(),
	}

	s.emitExportEvent(ctx, filter, result)
	return result, nil
}

func normalizeExportFormat(format ExportFormat) ExportFormat {
	switch strings.ToLower(string(format)) {
	case "json":
		return ExportFormatJSON
	default:
		return ExportFormatCSV
	}
}

func buildExportSteps(records []modelworkflow.WorkflowStepRecord) []ExportStep {
	if len(records) == 0 {
		return nil
	}
	steps := make([]ExportStep, 0, len(records))
	for _, rec := range records {
		subjectID := ""
		if rec.SubjectUUID != uuid.Nil {
			subjectID = rec.SubjectUUID.String()
		}
		steps = append(steps, ExportStep{
			StepID:           rec.StepID,
			Type:             rec.Type,
			State:            rec.State,
			SubjectType:      rec.SubjectType,
			SubjectID:        subjectID,
			Attempts:         int(rec.Attempt),
			ToolGrantVersion: rec.ToolGrantVer,
			LastTransitionAt: rec.LastTransition,
			LastError:        rec.FailureReason,
		})
	}
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].LastTransitionAt.Before(steps[j].LastTransitionAt)
	})
	return steps
}

func (s *Service) emitExportEvent(ctx context.Context, filter ExportFilter, result ExportResult) {
	if s == nil || s.em == nil {
		return
	}

	payload := map[string]any{
		"format":        string(result.Format),
		"row_count":     len(result.Rows),
		"generated":     result.GeneratedAt.Format(time.RFC3339),
		"step_included": filter.IncludeStepDetails,
	}
	if filter.DefinitionUUID != nil {
		payload["definition_uuid"] = filter.DefinitionUUID.String()
	}
	if filter.State != "" {
		payload["state"] = strings.ToLower(filter.State)
	}

	event := newWorkflowEvent(
		filter.TenantUUID,
		uuid.Nil,
		"workflow.reporting.export.generated",
		fmt.Sprintf("workflow export generated (%d rows)", len(result.Rows)),
		payload,
	)
	s.em.emit(ctx, event)
}
