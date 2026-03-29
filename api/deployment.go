package api

// DeploymentDTO is the API representation of a deployment version.
type DeploymentDTO struct {
	ID        string `json:"id"`
	AppID     string `json:"app_id"`
	Version   int    `json:"version"`
	Slot      string `json:"slot"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}
