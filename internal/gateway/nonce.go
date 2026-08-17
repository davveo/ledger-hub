package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type memoryNonce struct {
	mu   sync.Mutex
	seen map[string]int64
}

func newMemoryNonce() *memoryNonce {
	return &memoryNonce{seen: map[string]int64{}}
}

func (m *memoryNonce) Consume(_ context.Context, clientID, nonce string, ttl time.Duration) error {
	if clientID == "" || nonce == "" {
		return domain.ErrInvalidParam
	}
	key := clientID + "|" + nonce
	now := time.Now().Unix()
	exp := now + int64(ttl/time.Second)
	if exp <= now {
		exp = now + 300
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, until := range m.seen {
		if until < now {
			delete(m.seen, k)
		}
	}
	if _, ok := m.seen[key]; ok {
		return domain.ErrReplay
	}
	m.seen[key] = exp
	return nil
}

func (m *memoryNonce) DeleteBefore(_ context.Context, before time.Time) (int64, error) {
	cut := before.Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for k, until := range m.seen {
		if until < cut {
			delete(m.seen, k)
			n++
		}
	}
	return n, nil
}
