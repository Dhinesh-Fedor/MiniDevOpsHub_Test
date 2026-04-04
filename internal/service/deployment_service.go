package service

import (
	"sync"

	"github.com/minidevopshub/minidevopshub/internal/deployment"
)

type DeployConfig struct {
	ProjectID     string `json:"project_id"`
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch"`
	WorkerID      string `json:"worker_id"`
	WorkerIP      string `json:"worker_ip"`
	ImageName     string `json:"image_name"`
	ContainerName string `json:"container_name"`
	WorkspaceDir  string `json:"workspace_dir"`
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
}

type DeploymentService interface {
	CreateDeployment(d *deployment.Deployment) error
	ListDeployments(appID string) ([]*deployment.Deployment, error)
	GetActiveDeployment(appID string) (*deployment.Deployment, error)
	Rollback(appID string) error
	RecordLastConfig(appID string, cfg *DeployConfig) error
	GetLastConfig(appID string) (*DeployConfig, error)
	DeleteLastConfig(appID string) error
}

type InMemoryDeploymentService struct {
	mu          sync.RWMutex
	deployments map[string][]*deployment.Deployment // appID -> deployments
	lastConfigs map[string]*DeployConfig
}

func NewInMemoryDeploymentService() *InMemoryDeploymentService {
	return &InMemoryDeploymentService{
		deployments: make(map[string][]*deployment.Deployment),
		lastConfigs: make(map[string]*DeployConfig),
	}
}

func (s *InMemoryDeploymentService) CreateDeployment(d *deployment.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deployments[d.AppID] = append(s.deployments[d.AppID], d)
	return nil
}

func (s *InMemoryDeploymentService) ListDeployments(appID string) ([]*deployment.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deployments[appID], nil
}

func (s *InMemoryDeploymentService) GetActiveDeployment(appID string) (*deployment.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.deployments[appID]
	for _, d := range list {
		if d.Status == "live" {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

func (s *InMemoryDeploymentService) Rollback(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// For demo: just swap blue/green status
	list := s.deployments[appID]
	for _, d := range list {
		if d.Status == "live" {
			d.Status = "inactive"
		} else if d.Status == "inactive" {
			d.Status = "live"
		}
	}
	return nil
}

func (s *InMemoryDeploymentService) RecordLastConfig(appID string, cfg *DeployConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastConfigs[appID] = cfg
	return nil
}

func (s *InMemoryDeploymentService) GetLastConfig(appID string) (*DeployConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.lastConfigs[appID]
	if !ok {
		return nil, ErrNotFound
	}
	return cfg, nil
}

func (s *InMemoryDeploymentService) DeleteLastConfig(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lastConfigs, appID)
	return nil
}
