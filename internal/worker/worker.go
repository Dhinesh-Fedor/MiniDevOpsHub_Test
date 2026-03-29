package worker

// Worker represents a worker node in the system.
type Worker struct {
	ID       string
	Name     string
	IP       string
	Status   string // active, free, busy
}
