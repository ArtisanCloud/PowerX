package main

import "testing"

func TestHasDuplicateRunStateTaskID(t *testing.T) {
	if !hasDuplicateRunStateTaskID(map[string]any{
		"tasks": []any{
			map[string]any{"task_id": "analysis", "status": "running"},
			map[string]any{"task_id": "analysis", "status": "completed"},
		},
	}) {
		t.Fatal("expected duplicate task id to require repair")
	}
	if hasDuplicateRunStateTaskID(map[string]any{
		"tasks": []any{
			map[string]any{"task_id": "analysis", "status": "completed"},
			map[string]any{"task_id": "synthesis", "status": "completed"},
		},
	}) {
		t.Fatal("unique task ids must not require repair")
	}
}
