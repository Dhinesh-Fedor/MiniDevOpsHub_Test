package storage

import (
	"encoding/json"
	"os"
)

type FileStorage struct {
	path string
}

func NewFileStorage(path string) *FileStorage {
	return &FileStorage{path: path}
}

func (fs *FileStorage) Save(data interface{}) error {
	f, err := os.Create(fs.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}

func (fs *FileStorage) Load(data interface{}) error {
	f, err := os.Open(fs.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(data)
}
