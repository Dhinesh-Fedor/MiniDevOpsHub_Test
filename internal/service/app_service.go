package service

import (
	"github.com/minidevopshub/minidevopshub/internal/app"
)

type AppService interface {
	CreateApp(a *app.App) error
	GetApp(id string) (*app.App, error)
	ListApps() ([]*app.App, error)
	DeleteApp(id string) error
}

// InMemoryAppService is a simple in-memory implementation for demo purposes.
type InMemoryAppService struct {
	apps map[string]*app.App
}

func NewInMemoryAppService() *InMemoryAppService {
	return &InMemoryAppService{apps: make(map[string]*app.App)}
}

func (s *InMemoryAppService) CreateApp(a *app.App) error {
	s.apps[a.ID] = a
	return nil
}

func (s *InMemoryAppService) GetApp(id string) (*app.App, error) {
	a, ok := s.apps[id]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

func (s *InMemoryAppService) ListApps() ([]*app.App, error) {
	apps := []*app.App{}
	for _, a := range s.apps {
		apps = append(apps, a)
	}
	return apps, nil
}

func (s *InMemoryAppService) DeleteApp(id string) error {
	delete(s.apps, id)
	return nil
}

var ErrNotFound = &NotFoundError{"not found"}

type NotFoundError struct{ msg string }

func (e *NotFoundError) Error() string { return e.msg }
