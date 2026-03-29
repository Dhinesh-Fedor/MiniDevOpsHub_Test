package app

// App represents a deployed application in the system.
type App struct {
	ID          string
	Name        string
	RepoURL     string
	Branch      string
	LastCommit  string
	CommitTime  string
	WorkerID    string
	Status      string // Created, Deploying, Live
	ActiveSlot  string // blue or green
}
