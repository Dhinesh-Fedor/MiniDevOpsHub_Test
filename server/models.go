package main

type App struct {
	ProjectID      string `json:"project_id"`
	Name           string `json:"name"`
	RepoURL        string `json:"repo_url"`
	Branch         string `json:"branch"`
	WorkerID       string `json:"worker_id"`
	WorkerName     string `json:"worker_name"`
	WorkerIP       string `json:"worker_ip"`
	Status         string `json:"status"`
	Port           int    `json:"port"`
	LiveURL        string `json:"live_url"`
	WorkspaceDir   string `json:"workspace_dir,omitempty"`
	ImageName      string `json:"image_name,omitempty"`
	ContainerName  string `json:"container_name,omitempty"`
	LastDeployment string `json:"last_deployment,omitempty"`
}

type DeployRequest struct {
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch"`
	WorkerID  string `json:"worker_id"`
	ProjectID string `json:"project_id,omitempty"`
}

type DeployResponse struct {
	ProjectID string `json:"project_id"`
	Worker    string `json:"worker,omitempty"`
	Port      string `json:"port,omitempty"`
	LiveURL   string `json:"live_url"`
}

type CleanRequest struct {
	ProjectID string `json:"project_id"`
}

type RollbackRequest struct {
	ProjectID string `json:"project_id"`
}

var projects = map[string]*App{}
