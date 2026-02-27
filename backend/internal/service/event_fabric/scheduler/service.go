package scheduler

import (
	"fmt"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

type ComputeNextRunInput struct {
	CronExpr      string
	Timezone      string
	MisfirePolicy string
	Now           time.Time
	LastRunAt     *time.Time
	PrevNextRunAt *time.Time
}

type ComputeNextRunResult struct {
	NextRunAt    *time.Time
	ShouldRunNow bool
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) ComputeNextRun(input ComputeNextRunInput) (*ComputeNextRunResult, error) {
	location, err := resolveLocation(input.Timezone)
	if err != nil {
		return nil, err
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowInLoc := now.In(location)

	spec, err := parseCronSpec(input.CronExpr)
	if err != nil {
		return nil, err
	}

	policy := normalizeMisfirePolicy(input.MisfirePolicy)
	result := &ComputeNextRunResult{}

	if input.PrevNextRunAt != nil && !input.PrevNextRunAt.IsZero() {
		dueInLoc := input.PrevNextRunAt.In(location)
		if dueInLoc.Before(nowInLoc) {
			switch policy {
			case eventfabricmodel.ScheduledTaskMisfireFireNow:
				result.ShouldRunNow = true
				nextFromNow, nextErr := spec.next(nowInLoc)
				if nextErr != nil {
					return nil, nextErr
				}
				nextUTC := nextFromNow.UTC()
				result.NextRunAt = &nextUTC
				return result, nil
			case eventfabricmodel.ScheduledTaskMisfireCatchUp:
				result.ShouldRunNow = true
				nextAfterDue, nextErr := spec.next(dueInLoc)
				if nextErr != nil {
					return nil, nextErr
				}
				nextUTC := nextAfterDue.UTC()
				result.NextRunAt = &nextUTC
				return result, nil
			case eventfabricmodel.ScheduledTaskMisfireSkip:
				fallthrough
			default:
				nextFromNow, nextErr := spec.next(nowInLoc)
				if nextErr != nil {
					return nil, nextErr
				}
				nextUTC := nextFromNow.UTC()
				result.NextRunAt = &nextUTC
				return result, nil
			}
		}
	}

	base := nowInLoc
	if input.LastRunAt != nil && !input.LastRunAt.IsZero() {
		lastRunInLoc := input.LastRunAt.In(location)
		if lastRunInLoc.After(base) {
			base = lastRunInLoc
		}
	}
	nextAt, err := spec.next(base)
	if err != nil {
		return nil, err
	}
	nextUTC := nextAt.UTC()
	result.NextRunAt = &nextUTC
	return result, nil
}

func resolveLocation(timezone string) (*time.Location, error) {
	tz := strings.TrimSpace(timezone)
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	return loc, nil
}

func normalizeMisfirePolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	switch policy {
	case eventfabricmodel.ScheduledTaskMisfireFireNow,
		eventfabricmodel.ScheduledTaskMisfireCatchUp,
		eventfabricmodel.ScheduledTaskMisfireSkip:
		return policy
	default:
		return eventfabricmodel.ScheduledTaskMisfireSkip
	}
}
