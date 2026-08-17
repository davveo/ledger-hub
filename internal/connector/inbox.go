package connector

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/davveo/ledger-hub/internal/application"
	"github.com/davveo/ledger-hub/internal/domain"
	"github.com/davveo/ledger-hub/pkg/client"
)

const maxInboxAttempts = 5

type Processor struct {
	inbox    domain.InboxRepository
	orderCli *client.Client
	payCli   *client.Client
}

func NewProcessor(inbox domain.InboxRepository, orderCli, payCli *client.Client) *Processor {
	if inbox == nil {
		inbox = NewMemoryInbox()
	}
	return &Processor{inbox: inbox, orderCli: orderCli, payCli: payCli}
}

func (p *Processor) Inbox() domain.InboxRepository { return p.inbox }

func (p *Processor) Ingest(ctx context.Context, messageID, topic string, schemaVersion int, payload []byte) (*domain.InboxMessage, error) {
	if schemaVersion <= 0 {
		schemaVersion = 1
	}
	if messageID == "" {
		messageID = "msg_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	msg := &domain.InboxMessage{
		MessageID:     messageID,
		Topic:         topic,
		SchemaVersion: schemaVersion,
		Payload:       string(payload),
		Status:        domain.InboxPending,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	_, err := p.inbox.InsertIfAbsent(ctx, msg)
	if err != nil {
		return nil, err
	}
	stored, err := p.inbox.Get(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if stored.Status == domain.InboxDone {
		return stored, nil
	}
	return p.process(ctx, stored)
}

func (p *Processor) Replay(ctx context.Context, messageID string) (*domain.InboxMessage, error) {
	msg, err := p.inbox.Get(ctx, messageID)
	if err != nil {
		return nil, err
	}
	msg.Status = domain.InboxPending
	msg.LastError = ""
	next := time.Now().UTC()
	msg.NextRetryAt = &next
	if err := p.inbox.Update(ctx, msg); err != nil {
		return nil, err
	}
	return p.process(ctx, msg)
}

func (p *Processor) DrainDue(ctx context.Context, limit int) (int, error) {
	list, err := p.inbox.ListDue(ctx, time.Now().UTC(), limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range list {
		if m.Status == domain.InboxDone || m.Status == domain.InboxDead {
			continue
		}
		if _, err := p.process(ctx, m); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

func (p *Processor) process(ctx context.Context, msg *domain.InboxMessage) (*domain.InboxMessage, error) {
	var ev MQEvent
	if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
		msg.Status = domain.InboxDead
		msg.LastError = err.Error()
		msg.Attempts++
		_ = p.inbox.Update(ctx, msg)
		return msg, err
	}
	if ev.Topic == "" {
		ev.Topic = msg.Topic
	}
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = msg.SchemaVersion
	}
	msg.Attempts++
	_, err := ApplyMQ(ctx, p.orderCli, p.payCli, ev)
	if err != nil {
		msg.LastError = err.Error()
		if msg.Attempts >= maxInboxAttempts {
			msg.Status = domain.InboxDead
		} else {
			msg.Status = domain.InboxRetry
			next := time.Now().UTC().Add(application.BackoffDuration(msg.Attempts-1, time.Second, 30*time.Second))
			msg.NextRetryAt = &next
		}
		_ = p.inbox.Update(ctx, msg)
		return msg, err
	}
	msg.Status = domain.InboxDone
	msg.LastError = ""
	_ = p.inbox.Update(ctx, msg)
	return msg, nil
}

type MemoryInbox struct {
	mu    sync.Mutex
	items map[string]*domain.InboxMessage
}

func NewMemoryInbox() *MemoryInbox {
	return &MemoryInbox{items: map[string]*domain.InboxMessage{}}
}

func (m *MemoryInbox) InsertIfAbsent(_ context.Context, msg *domain.InboxMessage) (bool, error) {
	if msg == nil || msg.MessageID == "" {
		return false, domain.ErrInvalidParam
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[msg.MessageID]; ok {
		return false, nil
	}
	cp := *msg
	m.items[msg.MessageID] = &cp
	return true, nil
}

func (m *MemoryInbox) Get(_ context.Context, id string) (*domain.InboxMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it := m.items[id]
	if it == nil {
		return nil, domain.ErrNotFound
	}
	cp := *it
	return &cp, nil
}

func (m *MemoryInbox) Update(_ context.Context, msg *domain.InboxMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items[msg.MessageID] == nil {
		return domain.ErrNotFound
	}
	cp := *msg
	cp.UpdatedAt = time.Now().UTC()
	m.items[msg.MessageID] = &cp
	return nil
}

func (m *MemoryInbox) List(_ context.Context, status string, limit int) ([]*domain.InboxMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.InboxMessage
	for _, it := range m.items {
		if status != "" && it.Status != status {
			continue
		}
		cp := *it
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryInbox) ListDue(_ context.Context, now time.Time, limit int) ([]*domain.InboxMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.InboxMessage
	for _, it := range m.items {
		if it.Status != domain.InboxPending && it.Status != domain.InboxRetry {
			continue
		}
		if it.NextRetryAt != nil && it.NextRetryAt.After(now) {
			continue
		}
		cp := *it
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
