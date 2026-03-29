package api

// AppDTO is the API representation of an app.
type AppDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	LastCommit string `json:"last_commit"`
	CommitTime string `json:"commit_time"`
	WorkerID   string `json:"worker_id"`
	Status     string `json:"status"`
	ActiveSlot string `json:"active_slot"`
}
