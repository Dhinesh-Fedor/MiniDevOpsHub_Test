package main

type App struct {
	Name        string
	Repo        string
	Worker      string
	WorkerIP    string
	Version     int
	Status      string
	ActiveColor string // blue or green
}

var apps = map[string]*App{}
var logs = map[string][]string{}
var workers = map[string]string{
	"w1": "10.0.0.11",
	"w2": "10.0.0.12",
	"w3": "10.0.0.13",
}
