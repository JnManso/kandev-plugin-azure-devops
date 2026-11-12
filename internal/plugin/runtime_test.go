package plugin

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func TestRuntimeAcknowledgesEvents(t *testing.T) {
	if err := NewRuntime().OnEvent(context.Background(), &pluginsdk.Event{EventType: "task.created"}); err != nil {
		t.Fatalf("OnEvent() error = %v, want nil", err)
	}
}

func TestRuntimeAnswersUndeclaredWebhooksWithNotFound(t *testing.T) {
	response, err := NewRuntime().HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "unknown"})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if response.Status != 404 {
		t.Fatalf("HandleWebhook() status = %d, want 404", response.Status)
	}
}

func TestRuntimeRetriesTaskDeletedBeforeTheHostIsReady(t *testing.T) {
	err := NewRuntime().OnEvent(context.Background(), &pluginsdk.Event{EventType: TaskDeletedEvent, Payload: map[string]any{"task_id": "t"}})
	if err == nil {
		t.Fatal("OnEvent() error = nil, want a retryable error before SetHost")
	}
}
