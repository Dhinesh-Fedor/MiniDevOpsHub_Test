package api

// WorkerDTO is the API representation of a worker node.
type WorkerDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"`
}
