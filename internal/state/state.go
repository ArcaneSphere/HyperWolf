package state

import (
	"sync"
	"time"
)

type AppState struct {
	mu          sync.RWMutex
	Node        string
	ConnectedAt time.Time
}

func New() *AppState {
	return &AppState{}
}

func (s *AppState) SetNode(node string) {
	s.mu.Lock()
	s.Node = node
	s.ConnectedAt = time.Now()
	s.mu.Unlock()
}

func (s *AppState) ClearNode() {
	s.mu.Lock()
	s.Node = ""
	s.ConnectedAt = time.Time{}
	s.mu.Unlock()
}

func (s *AppState) GetNode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Node
}

func (s *AppState) GetConnectedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConnectedAt
}

func (s *AppState) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Node != ""
}
