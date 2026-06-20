package app

import (
	"net/http"
	"sync"
	"time"
)

type TicketStore struct {
	mu        sync.RWMutex
	ticket    string
	updatedAt time.Time
}

func (s *TicketStore) Capture(r *http.Request) bool {
	ticket := r.Header.Get("X-HC-USER-TICKET")
	if ticket == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticket == ticket {
		return false
	}
	s.ticket = ticket
	s.updatedAt = time.Now()
	return true
}

func (s *TicketStore) Get() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ticket, s.ticket != ""
}

func (s *TicketStore) UpdatedAt() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.updatedAt.IsZero() {
		return nil
	}
	value := s.updatedAt
	return &value
}
