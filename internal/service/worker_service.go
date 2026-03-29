package service

import (
	"github.com/minidevopshub/minidevopshub/internal/worker"
)

type WorkerService interface {
	CreateWorker(w *worker.Worker) error
	GetWorker(id string) (*worker.Worker, error)
	ListWorkers() ([]*worker.Worker, error)
}

type InMemoryWorkerService struct {
	workers map[string]*worker.Worker
}

func NewInMemoryWorkerService() *InMemoryWorkerService {
	return &InMemoryWorkerService{workers: make(map[string]*worker.Worker)}
}

func (s *InMemoryWorkerService) CreateWorker(w *worker.Worker) error {
	s.workers[w.ID] = w
	return nil
}

func (s *InMemoryWorkerService) GetWorker(id string) (*worker.Worker, error) {
	w, ok := s.workers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return w, nil
}

func (s *InMemoryWorkerService) ListWorkers() ([]*worker.Worker, error) {
	workers := []*worker.Worker{}
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	return workers, nil
}
