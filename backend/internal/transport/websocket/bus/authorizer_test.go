package bus

import (
	"context"
	"errors"
	"testing"
)

func TestAuthorizeAllowsDynamicTopicForSameTenant(t *testing.T) {
	resetPublishRegistry()

	tenantUUID := "tenant-authorizer"
	topic := "custom.progress"
	RegisterPublishTopics(tenantUUID, []string{topic})

	authorizer := &DefaultAuthorizer{}
	client := &Client{TenantUUID: tenantUUID, MemberID: 1}

	if err := authorizer.Authorize(context.Background(), client, topic); err != nil {
		t.Fatalf("expected dynamic topic allowed, got err=%v", err)
	}
}

func TestAuthorizeRejectsDynamicBypassForKnowledgeTopic(t *testing.T) {
	resetPublishRegistry()

	tenantUUID := "tenant-authorizer"
	RegisterPublishTopics(tenantUUID, []string{TopicKnowledgeIngestionJob, TopicKnowledgeCorpusCheck})

	authorizer := &DefaultAuthorizer{}
	client := &Client{TenantUUID: tenantUUID, MemberID: 1}

	err := authorizer.Authorize(context.Background(), client, TopicKnowledgeIngestionJob)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied for ingestion topic, got err=%v", err)
	}

	err = authorizer.Authorize(context.Background(), client, TopicKnowledgeCorpusCheck)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied for corpus topic, got err=%v", err)
	}
}

