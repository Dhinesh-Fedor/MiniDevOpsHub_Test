package service

import "sync"

type LogService interface {
	AppendLog(appID string, slot string, lines []string) error
	GetLogs(appID string, slot string) ([]string, error)
	ClearLogs(appID string) error
	ReplaceLogs(appID string, slot string, lines []string) error
}

type InMemoryLogService struct {
	mu   sync.RWMutex
	logs map[string]map[string][]string // appID -> slot -> lines
}

func NewInMemoryLogService() *InMemoryLogService {
	return &InMemoryLogService{logs: make(map[string]map[string][]string)}
}

func (s *InMemoryLogService) AppendLog(appID string, slot string, lines []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs[appID] == nil {
		s.logs[appID] = make(map[string][]string)
	}
	s.logs[appID][slot] = append(s.logs[appID][slot], lines...)
	return nil
}

func (s *InMemoryLogService) GetLogs(appID string, slot string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.logs[appID] == nil {
		return nil, nil
	}
	return s.logs[appID][slot], nil
}

func (s *InMemoryLogService) ClearLogs(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.logs, appID)
	return nil
}

func (s *InMemoryLogService) ReplaceLogs(appID string, slot string, lines []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs[appID] == nil {
		s.logs[appID] = make(map[string][]string)
	}
	copyLines := make([]string, len(lines))
	copy(copyLines, lines)
	s.logs[appID][slot] = copyLines
	return nil
}
