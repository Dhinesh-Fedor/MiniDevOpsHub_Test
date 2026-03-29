package service

type LogService interface {
	AppendLog(appID string, slot string, lines []string) error
	GetLogs(appID string, slot string) ([]string, error)
}

type InMemoryLogService struct {
	logs map[string]map[string][]string // appID -> slot -> lines
}

func NewInMemoryLogService() *InMemoryLogService {
	return &InMemoryLogService{logs: make(map[string]map[string][]string)}
}

func (s *InMemoryLogService) AppendLog(appID string, slot string, lines []string) error {
	if s.logs[appID] == nil {
		s.logs[appID] = make(map[string][]string)
	}
	s.logs[appID][slot] = append(s.logs[appID][slot], lines...)
	return nil
}

func (s *InMemoryLogService) GetLogs(appID string, slot string) ([]string, error) {
	if s.logs[appID] == nil {
		return nil, nil
	}
	return s.logs[appID][slot], nil
}
