package backup_ops

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidStateTransition = errors.New("invalid job state transition")
	ErrInvalidJobState        = errors.New("invalid job state")
)

type JobState string

type JobEvent string

const (
	JobStatePending JobState = "pending"
	JobStateRunning JobState = "running"
	JobStateSuccess JobState = "success"
	JobStateFailed  JobState = "failed"

	JobEventStart   JobEvent = "start"
	JobEventSucceed JobEvent = "succeed"
	JobEventFail    JobEvent = "fail"
	JobEventReset   JobEvent = "reset"
)

type JobStateMachine struct{}

func NewJobStateMachine() *JobStateMachine {
	return &JobStateMachine{}
}

func (m *JobStateMachine) Next(current JobState, event JobEvent) (JobState, error) {
	if !m.isKnownState(current) {
		return "", fmt.Errorf("%w: %s", ErrInvalidJobState, current)
	}

	switch current {
	case JobStatePending:
		switch event {
		case JobEventStart:
			return JobStateRunning, nil
		case JobEventReset:
			return JobStatePending, nil
		}
	case JobStateRunning:
		switch event {
		case JobEventSucceed:
			return JobStateSuccess, nil
		case JobEventFail:
			return JobStateFailed, nil
		}
	case JobStateSuccess, JobStateFailed:
		if event == JobEventReset {
			return JobStatePending, nil
		}
	}

	return "", fmt.Errorf("%w: %s --(%s)", ErrInvalidStateTransition, current, event)
}

func (m *JobStateMachine) isKnownState(state JobState) bool {
	switch state {
	case JobStatePending, JobStateRunning, JobStateSuccess, JobStateFailed:
		return true
	default:
		return false
	}
}

func MapJobError(err error) (code, message string) {
	if err == nil {
		return "OK", ""
	}
	if errors.Is(err, ErrInvalidStateTransition) || errors.Is(err, ErrInvalidJobState) {
		return "INVALID_STATE", err.Error()
	}
	return "INTERNAL_ERROR", err.Error()
}
