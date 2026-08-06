package id

import "github.com/google/uuid"

type Generator interface {
	New() string
}

type UUID struct{}

func (UUID) New() string {
	return uuid.NewString()
}
