package store

import (
	"errors"
	"sync"
	"time"
)

// defaultDuration the default TTL for items in the cache.
const defaultDuration = time.Duration(time.Hour)

// storeConfig the config for the data store.
type storeConfig struct {
	// TTL the time to life for entries in the store.
	TTL time.Duration
}

// Store the interface for a store / cache instance.
type Store interface {
	Get(Key) ([]byte, error)
	Set(Key, []byte) error
	Remove(Key) error
}

// Item a single item in the store.
type item struct {
	Expires time.Time
	Value   []byte
}

// Expired determines if the item is expired.
func (i *item) Expired() bool {
	return i.Expires.Before(time.Now().UTC())
}

// Key a representation of a key.
type Key string

// store the storage server.
// This could be improved by flushing items to disk frequency.
// At minute all items are lost when the server is destroyed.
type store struct {
	sync.RWMutex
	// items the store of items.
	items map[Key]item
	// config the store for the config.
	config storeConfig
}

// errNotFound error returned when an item is not found in the store.
var errNotFound = errors.New("item not found")

// emptyData a byte slice containing no data.
var emptyData = []byte("")

// Get gets an item from the store.
func (s *store) Get(k Key) ([]byte, error) {
	s.RLock()
	i, ok := s.items[k]
	s.RUnlock()

	if !ok {
		return emptyData, errNotFound
	}
	if i.Expired() {
		s.Remove(k)
		return emptyData, errNotFound
	}

	return i.Value, nil
}

// Set sets an item in the store.
func (s *store) Set(k Key, v []byte) error {
	i := item{
		Expires: time.Now().UTC().Add(s.config.TTL),
		Value:   v,
	}

	s.Lock()
	defer s.Unlock()
	s.items[k] = i

	return nil
}

// Remove removes an item from the store.
func (s *store) Remove(k Key) error {
	s.Lock()
	defer s.Unlock()
	delete(s.items, k)

	return nil
}

// New returns a new store instance.
func new(config storeConfig) Store {
	return &store{items: make(map[Key]item), config: config}
}
