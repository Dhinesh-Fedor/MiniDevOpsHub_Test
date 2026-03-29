package api

// LogDTO is the API representation of logs.
type LogDTO struct {
	AppID     string `json:"app_id"`
	Version   int    `json:"version"`
	Slot      string `json:"slot"`
	Lines     []string `json:"lines"`
}
