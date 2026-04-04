package worker

// Worker represents a worker node in the system.
type Worker struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"` // active, free, busy
}
