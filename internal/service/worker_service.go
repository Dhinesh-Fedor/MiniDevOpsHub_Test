package service

import (
	"sync"

	"github.com/minidevopshub/minidevopshub/internal/worker"
)

type WorkerService interface {
	CreateWorker(w *worker.Worker) error
	GetWorker(id string) (*worker.Worker, error)
	ListWorkers() ([]*worker.Worker, error)
}

type InMemoryWorkerService struct {
	mu      sync.RWMutex
	workers map[string]*worker.Worker
}

func NewInMemoryWorkerService() *InMemoryWorkerService {
	return &InMemoryWorkerService{workers: make(map[string]*worker.Worker)}
}

func (s *InMemoryWorkerService) CreateWorker(w *worker.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[w.ID] = w
	return nil
}

func (s *InMemoryWorkerService) GetWorker(id string) (*worker.Worker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return w, nil
}

func (s *InMemoryWorkerService) ListWorkers() ([]*worker.Worker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workers := []*worker.Worker{}
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	return workers, nil
}

func (s *InMemoryWorkerService) UpdateWorker(w *worker.Worker) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers[w.ID] = w
	return nil
}

func (s *InMemoryWorkerService) DeleteWorker(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, id)
	return nil
}
