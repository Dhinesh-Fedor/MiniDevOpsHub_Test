package worker

// Worker represents a worker node in the system.
type Worker struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Status     string `json:"status"` // active, inactive
	ActiveJobs int    `json:"active_jobs"`
}

// IsAvailable returns true if worker has no active jobs
func (w *Worker) IsAvailable() bool {
	return w.ActiveJobs == 0
}
