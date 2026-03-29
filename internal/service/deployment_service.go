package service

import (
	"github.com/minidevopshub/minidevopshub/internal/deployment"
)

type DeploymentService interface {
	CreateDeployment(d *deployment.Deployment) error
	ListDeployments(appID string) ([]*deployment.Deployment, error)
	GetActiveDeployment(appID string) (*deployment.Deployment, error)
	Rollback(appID string) error
}

type InMemoryDeploymentService struct {
	deployments map[string][]*deployment.Deployment // appID -> deployments
}

func NewInMemoryDeploymentService() *InMemoryDeploymentService {
	return &InMemoryDeploymentService{deployments: make(map[string][]*deployment.Deployment)}
}

func (s *InMemoryDeploymentService) CreateDeployment(d *deployment.Deployment) error {
	s.deployments[d.AppID] = append(s.deployments[d.AppID], d)
	return nil
}

func (s *InMemoryDeploymentService) ListDeployments(appID string) ([]*deployment.Deployment, error) {
	return s.deployments[appID], nil
}

func (s *InMemoryDeploymentService) GetActiveDeployment(appID string) (*deployment.Deployment, error) {
	list := s.deployments[appID]
	for _, d := range list {
		if d.Status == "live" {
			return d, nil
		}
	}
	return nil, ErrNotFound
}

func (s *InMemoryDeploymentService) Rollback(appID string) error {
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
