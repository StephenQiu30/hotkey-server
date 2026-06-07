package objectstorage

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store for testing.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string]*memoryObject
}

type memoryObject struct {
	obj  Object
	data []byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		objects: make(map[string]*memoryObject),
	}
}

func (s *MemoryStore) Put(_ context.Context, obj Object, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.objects[obj.Key]; exists {
		return ErrAlreadyExists
	}

	s.objects[obj.Key] = &memoryObject{
		obj:  obj,
		data: data,
	}
	return nil
}

func (s *MemoryStore) Get(_ context.Context, key string) (Object, io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	memObj, exists := s.objects[key]
	if !exists {
		return Object{}, nil, ErrNotFound
	}

	return memObj.obj, io.NopCloser(bytes.NewReader(memObj.data)), nil
}

func (s *MemoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.objects[key]; !exists {
		return ErrNotFound
	}

	delete(s.objects, key)
	return nil
}

func (s *MemoryStore) Head(_ context.Context, key string) (Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	memObj, exists := s.objects[key]
	if !exists {
		return Object{}, ErrNotFound
	}

	return memObj.obj, nil
}

func (s *MemoryStore) ListExpired(_ context.Context, _ string, before time.Time) ([]Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expired []Object
	for _, memObj := range s.objects {
		if memObj.obj.Metadata.ExpiresAt != nil && memObj.obj.Metadata.ExpiresAt.Before(before) {
			expired = append(expired, memObj.obj)
		}
	}
	return expired, nil
}

func (s *MemoryStore) ListByPrefix(_ context.Context, prefix string) ([]Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []Object
	for key, memObj := range s.objects {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			matched = append(matched, memObj.obj)
		}
	}
	return matched, nil
}
