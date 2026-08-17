package connector

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestInboxIdempotentAndReplay(t *testing.T) {
	box := NewMemoryInbox()
	ctx := context.Background()
	payload, _ := json.Marshal(MQEvent{Topic: "order", Event: "unknown", OrderID: "O1", UserID: "u1", SchemaVersion: 1})
	p := NewProcessor(box, nil, nil)
	msg, err := p.Ingest(ctx, "m1", "order", 1, payload)
	if err == nil {
		t.Fatal("unknown event should fail")
	}
	if msg.Status != domain.InboxRetry && msg.Status != domain.InboxDead {
		t.Fatalf("status %s", msg.Status)
	}
	again, err := p.Ingest(ctx, "m1", "order", 1, payload)
	if again.MessageID != "m1" {
		t.Fatalf("idempotent id %s err=%v", again.MessageID, err)
	}
	if again.Attempts < 2 {
		t.Fatalf("second ingest should process again or keep attempts, got %d", again.Attempts)
	}
	got, err := p.Replay(ctx, "m1")
	if err == nil && got.Status == domain.InboxDone {
		t.Fatal("replay of bad event should still fail")
	}
	if got == nil || got.MessageID != "m1" {
		t.Fatalf("replay %+v err=%v", got, err)
	}
}
