package application

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/davveo/ledger-hub/internal/domain"
)

func NewInstanceID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])
}

type MemoryLease struct {
	mu    sync.Mutex
	items map[string]domain.JobLease
	Now   func() time.Time
}

func NewMemoryLease() *MemoryLease {
	return &MemoryLease{items: map[string]domain.JobLease{}, Now: func() time.Time { return time.Now().UTC() }}
}

func (m *MemoryLease) now() time.Time {
	if m != nil && m.Now != nil {
		return m.Now()
	}
	return time.Now().UTC()
}

func (m *MemoryLease) Acquire(_ context.Context, jobName, holder string, ttl time.Duration) (bool, error) {
	return m.claim(jobName, holder, ttl, false)
}

func (m *MemoryLease) Renew(_ context.Context, jobName, holder string, ttl time.Duration) (bool, error) {
	return m.claim(jobName, holder, ttl, true)
}

func (m *MemoryLease) claim(jobName, holder string, ttl time.Duration, renewOnly bool) (bool, error) {
	if jobName == "" || holder == "" {
		return false, domain.ErrInvalidParam
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.items[jobName]
	if !ok {
		if renewOnly {
			return false, nil
		}
		m.items[jobName] = domain.JobLease{JobName: jobName, Holder: holder, ExpiresAt: now.Add(ttl), UpdatedAt: now}
		return true, nil
	}
	if cur.Holder != holder && cur.ExpiresAt.After(now) {
		return false, nil
	}
	if renewOnly && cur.Holder != holder {
		return false, nil
	}
	m.items[jobName] = domain.JobLease{JobName: jobName, Holder: holder, ExpiresAt: now.Add(ttl), UpdatedAt: now}
	return true, nil
}

type Lease struct {
	repo       domain.JobLeaseRepository
	instanceID string
	ttl        time.Duration
}

func NewLease(repo domain.JobLeaseRepository, instanceID string, ttl time.Duration) *Lease {
	if instanceID == "" {
		instanceID = NewInstanceID()
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Lease{repo: repo, instanceID: instanceID, ttl: ttl}
}

func (l *Lease) InstanceID() string {
	if l == nil {
		return ""
	}
	return l.instanceID
}

func (l *Lease) Acquire(ctx context.Context, jobName string) bool {
	if l == nil || l.repo == nil {
		return true
	}
	ok, err := l.repo.Acquire(ctx, jobName, l.instanceID, l.ttl)
	return err == nil && ok
}

func (l *Lease) Hold(ctx context.Context, jobName string, fn func()) {
	if fn == nil {
		return
	}
	if l == nil || l.repo == nil {
		fn()
		return
	}
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(l.ttl / 3)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				_, _ = l.repo.Renew(ctx, jobName, l.instanceID, l.ttl)
			}
		}
	}()
	defer close(stop)
	fn()
}
