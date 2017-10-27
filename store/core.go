package store

import (
	"sync"
	"time"
)

// storeConfig the config for the data store.
type storeConfig struct{}

// Store the interface for a store / cache instance.
type Store interface {
	Get(Key) (interface{}, error)
	Set(Key, interface{}) error
}

// Item a single item in the store.
type Item struct {
	Expires time.Time
	Value   interface{}
}

// Key a representation of a key.
type Key string

// store the storage server.
type store struct {
	sync.RWMutex
	// items the store of items.
	items map[Key]Item
	// config the store for the config.
	config storeConfig
}

func (s *store) Get(k Key) (interface{}, error) { return nil, nil }
func (s *store) Set(k Key, v interface{}) error { return nil }

// New returns a new store instance.
func new(config storeConfig) Store {
	return &store{items: make(map[Key]Item), config: config}
}
