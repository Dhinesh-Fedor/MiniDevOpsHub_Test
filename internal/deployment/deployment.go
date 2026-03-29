package deployment

// Deployment represents a deployment version (blue/green) for an app.
type Deployment struct {
	ID         string
	AppID      string
	Version    int
	Slot       string // blue or green
	Status     string // deploying, live, failed
	CreatedAt  string
}
