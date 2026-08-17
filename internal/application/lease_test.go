package application

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLeaseAcquireRenewTakeover(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	lease := NewMemoryLease()
	lease.Now = func() time.Time { return now }
	ctx := context.Background()

	ok, err := lease.Acquire(ctx, "reconcile", "a", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("a acquire: ok=%v err=%v", ok, err)
	}
	ok, err = lease.Acquire(ctx, "reconcile", "b", 30*time.Second)
	if err != nil || ok {
		t.Fatalf("b should not steal live lease, ok=%v err=%v", ok, err)
	}
	ok, err = lease.Renew(ctx, "reconcile", "a", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("a renew: ok=%v err=%v", ok, err)
	}
	ok, err = lease.Renew(ctx, "reconcile", "b", 30*time.Second)
	if err != nil || ok {
		t.Fatalf("b renew should fail, ok=%v", ok)
	}
	now = now.Add(31 * time.Second)
	ok, err = lease.Acquire(ctx, "reconcile", "b", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("b takeover expired: ok=%v err=%v", ok, err)
	}
}

func TestBackoffDuration(t *testing.T) {
	if got := BackoffDuration(0, time.Second, 30*time.Second); got != time.Second {
		t.Fatalf("attempt0 got %s", got)
	}
	if got := BackoffDuration(1, time.Second, 30*time.Second); got != 2*time.Second {
		t.Fatalf("attempt1 got %s", got)
	}
	if got := BackoffDuration(10, time.Second, 8*time.Second); got != 8*time.Second {
		t.Fatalf("cap got %s", got)
	}
}
