package notification

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockRepo struct {
	mu            sync.Mutex
	notifications []Notification
	nextID        int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{nextID: 1}
}

func (m *mockRepo) List(ctx context.Context, unreadOnly bool, limit int) ([]Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []Notification
	for _, n := range m.notifications {
		if unreadOnly && n.IsRead {
			continue
		}
		res = append(res, n)
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}

func (m *mockRepo) CountUnread(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifications {
		if !n.IsRead {
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) Create(ctx context.Context, req CreateNotificationRequest) (*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	level := req.Level
	if level == "" {
		level = "info"
	}
	n := Notification{
		ID:        m.nextID,
		Type:      req.Type,
		Title:     req.Title,
		Message:   req.Message,
		Level:     level,
		IsRead:    false,
		Metadata:  req.Metadata,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	m.nextID++
	m.notifications = append(m.notifications, n)
	return &n, nil
}

func (m *mockRepo) CreateIfNotExists(ctx context.Context, req CreateNotificationRequest) (*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.notifications {
		if existing.Type == req.Type && existing.Title == req.Title {
			// Deduplicated
			return nil, nil
		}
	}

	level := req.Level
	if level == "" {
		level = "info"
	}
	n := Notification{
		ID:        m.nextID,
		Type:      req.Type,
		Title:     req.Title,
		Message:   req.Message,
		Level:     level,
		IsRead:    false,
		Metadata:  req.Metadata,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	m.nextID++
	m.notifications = append(m.notifications, n)
	return &n, nil
}

func (m *mockRepo) MarkAsRead(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.notifications {
		if m.notifications[i].ID == id {
			m.notifications[i].IsRead = true
		}
	}
	return nil
}

func (m *mockRepo) MarkAllAsRead(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.notifications {
		m.notifications[i].IsRead = true
	}
	return nil
}

func (m *mockRepo) Delete(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var updated []Notification
	for _, n := range m.notifications {
		if n.ID != id {
			updated = append(updated, n)
		}
	}
	m.notifications = updated
	return nil
}

func (m *mockRepo) CleanOld(ctx context.Context, days int) (int64, error) {
	return 0, nil
}

func TestService_BroadcastEvents(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(svc *Service)
		action        func(svc *Service)
		expectMessage bool
		checkContent  func(t *testing.T, msg string)
	}{
		{
			name: "Create broadcasts notification event",
			action: func(svc *Service) {
				_, err := svc.Create(CreateNotificationRequest{
					Type:    "alert",
					Title:   "Disk Full",
					Message: "Disk space low",
				})
				if err != nil {
					t.Fatalf("Create failed: %v", err)
				}
			},
			expectMessage: true,
			checkContent: func(t *testing.T, msg string) {
				if !strings.HasPrefix(msg, "event: notification\n") {
					t.Errorf("Expected event: notification, got: %s", msg)
				}
				if !strings.Contains(msg, `"title":"Disk Full"`) {
					t.Errorf("Expected message to contain title, got: %s", msg)
				}
			},
		},
		{
			name: "CreateIfNotExists new item broadcasts notification event",
			action: func(svc *Service) {
				_, err := svc.CreateIfNotExists(CreateNotificationRequest{
					Type:    "alert",
					Title:   "High Memory",
					Message: "RAM usage high",
				})
				if err != nil {
					t.Fatalf("CreateIfNotExists failed: %v", err)
				}
			},
			expectMessage: true,
			checkContent: func(t *testing.T, msg string) {
				if !strings.HasPrefix(msg, "event: notification\n") {
					t.Errorf("Expected event: notification, got: %s", msg)
				}
				if !strings.Contains(msg, `"title":"High Memory"`) {
					t.Errorf("Expected message to contain title, got: %s", msg)
				}
			},
		},
		{
			name: "CreateIfNotExists duplicate item does NOT broadcast",
			setup: func(svc *Service) {
				// Initial creation before listener registers
				_, _ = svc.CreateIfNotExists(CreateNotificationRequest{
					Type:  "alert",
					Title: "Duplicate Alert",
				})
			},
			action: func(svc *Service) {
				// Duplicate creation attempt
				_, _ = svc.CreateIfNotExists(CreateNotificationRequest{
					Type:  "alert",
					Title: "Duplicate Alert",
				})
			},
			expectMessage: false,
		},
		{
			name: "MarkAsRead broadcasts read event",
			action: func(svc *Service) {
				if err := svc.MarkAsRead(1); err != nil {
					t.Fatalf("MarkAsRead failed: %v", err)
				}
			},
			expectMessage: true,
			checkContent: func(t *testing.T, msg string) {
				if !strings.HasPrefix(msg, "event: read\n") {
					t.Errorf("Expected event: read, got: %s", msg)
				}
				if !strings.Contains(msg, `"ids":[1]`) || !strings.Contains(msg, `"all":false`) {
					t.Errorf("Expected read event payload for single id, got: %s", msg)
				}
			},
		},
		{
			name: "MarkAllAsRead broadcasts read event with all: true",
			action: func(svc *Service) {
				if err := svc.MarkAllAsRead(); err != nil {
					t.Fatalf("MarkAllAsRead failed: %v", err)
				}
			},
			expectMessage: true,
			checkContent: func(t *testing.T, msg string) {
				if !strings.HasPrefix(msg, "event: read\n") {
					t.Errorf("Expected event: read, got: %s", msg)
				}
				if !strings.Contains(msg, `"all":true`) {
					t.Errorf("Expected read event payload with all:true, got: %s", msg)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			svc := NewService(repo)

			if tt.setup != nil {
				tt.setup(svc)
			}

			client := &NotificationClient{
				Send: make(chan []byte, 10),
			}
			svc.Hub().Register(client)
			defer svc.Hub().Unregister(client)

			// Drain initial messages if any
			for len(client.Send) > 0 {
				<-client.Send
			}

			tt.action(svc)

			select {
			case msg := <-client.Send:
				if !tt.expectMessage {
					t.Fatalf("Expected no message broadcasted, but received: %s", string(msg))
				}
				if tt.checkContent != nil {
					tt.checkContent(t, string(msg))
				}
			case <-time.After(50 * time.Millisecond):
				if tt.expectMessage {
					t.Fatal("Expected broadcasted message, but timed out")
				}
			}
		})
	}
}
