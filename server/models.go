package main

type App struct {
	ProjectID      string   `json:"project_id"`
	Name           string   `json:"name"`
	RepoURL        string   `json:"repo_url"`
	Branch         string   `json:"branch"`
	AutoDeploy     bool     `json:"auto_deploy"`
	LastCommitHash string   `json:"last_commit_hash,omitempty"`
	PrevCommitHash string   `json:"prev_commit_hash,omitempty"`
	CommitHistory  []string `json:"commit_history,omitempty"`
	WorkerID       string   `json:"worker_id"`
	WorkerName     string   `json:"worker_name"`
	WorkerIP       string   `json:"worker_ip"`
	Status         string   `json:"status"`
	Port           int      `json:"port"`
	LiveURL        string   `json:"live_url"`
	WorkspaceDir   string   `json:"workspace_dir,omitempty"`
	ImageName      string   `json:"image_name,omitempty"`
	ContainerName  string   `json:"container_name,omitempty"`
	LastDeployment string   `json:"last_deployment,omitempty"`
}

type DeployRequest struct {
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	WorkerID   string `json:"worker_id"`
	ProjectID  string `json:"project_id,omitempty"`
	AutoDeploy *bool  `json:"auto_deploy,omitempty"`
}

type DeployResponse struct {
	ProjectID string `json:"project_id"`
	Worker    string `json:"worker,omitempty"`
	Port      string `json:"port,omitempty"`
	Status    string `json:"status,omitempty"`
	LiveURL   string `json:"live_url"`
	Revision  string `json:"revision,omitempty"`
}

type CleanRequest struct {
	ProjectID string `json:"project_id"`
}

type RollbackRequest struct {
	ProjectID string `json:"project_id"`
}

var projects = map[string]*App{}
