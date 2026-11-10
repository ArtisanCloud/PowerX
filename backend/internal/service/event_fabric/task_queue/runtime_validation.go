package taskqueue

import (
	"encoding/json"
	"strings"
	"time"
)

func validateRuntimeOperation(in RuntimeRequest) error {
	if in.Operation == "ack" {
		if in.Outcome != "completed" && in.Outcome != "failed" {
			return runtimeError(400, "TASK_QUEUE_RESULT_REQUIRED")
		}
	} else if in.Outcome != "" {
		return runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	invalid := func() error { return runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT") }
	if len(in.Metadata) > 32 {
		return invalid()
	}
	for key, value := range in.Metadata {
		if !runtimeValidPart(key, 128) || len(value) > 1<<20 || strings.HasPrefix(key, "runtime_host_") || strings.HasPrefix(key, "taskbus_") {
			return invalid()
		}
	}
	raw, err := json.Marshal(in.Metadata)
	if err != nil || len(raw) > 2<<20 {
		return invalid()
	}
	switch in.Operation {
	case "enqueue":
		if in.Message == nil || in.SubscriberID != "" || in.WorkerID != "" || in.MessageID != "" || in.ReceiptUUID != "" || in.RetryAt != nil || in.Reason != "" || len(in.Metadata) > 0 || in.MaxItems != 0 || in.WaitTimeoutMS != 0 {
			return invalid()
		}
		if len(in.Message.Headers) > 32 || len(in.Message.Metadata) > 32 || len(in.Message.TraceID) > 256 {
			return invalid()
		}
		for key, value := range in.Message.Headers {
			if !runtimeValidPart(key, 128) || len(value) > 2048 {
				return invalid()
			}
		}
	case "dequeue":
		if in.Message != nil || in.MessageID != "" || in.ReceiptUUID != "" || in.RetryAt != nil || in.Reason != "" || len(in.Metadata) > 0 {
			return invalid()
		}
	case "ack", "nack", "renew":
		if in.Message != nil || in.WorkerID != "" || in.MaxItems != 0 || in.WaitTimeoutMS != 0 {
			return invalid()
		}
		if in.Operation != "nack" && (in.RetryAt != nil || in.Reason != "") {
			return invalid()
		}
		if in.Operation == "renew" && len(in.Metadata) > 0 {
			return invalid()
		}
		if in.RetryAt != nil && in.RetryAt.After(time.Now().Add(24*time.Hour)) {
			return invalid()
		}
	default:
		return invalid()
	}
	return nil
}
