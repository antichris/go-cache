// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package cache implements a generic timed key-value in-memory data
// store.
package cache

import (
	"container/heap"
	"sync"
	"time"
)

// New Cache instance.
func New[K comparable, V any](defaultTTL time.Duration) *Cache[K, V] {
	c := &Cache[K, V]{
		d:    make(map[K]entry[K, V]),
		done: make(emptyChan),
		t:    time.NewTimer(indefinite),
		ttl:  defaultTTL,
	}
	go c.loop()

	return c
}

// NewByOf returns a new cache for values of the same type as
// sampleValue, indexed by keys of the same type as sampleKey.
func NewByOf[K comparable, V any](
	defaultTTL time.Duration,
	sampleKey K,
	sampleValue V,
) *Cache[K, V] {
	return New[K, V](defaultTTL)
}

// Cache of values.
type Cache[K comparable, V any] struct {
	d    map[K]entry[K, V]
	done emptyChan
	m    sync.Mutex
	t    *time.Timer
	th   timerHeap[K]
	ttl  time.Duration
}

// Has returns whether an item for given key is present in the cache.
//
// Unlike Touch, this does not extend the lifetime of the item.
func (c *Cache[K, T]) Has(key K) bool {
	c.m.Lock()
	defer c.m.Unlock()
	_, found := c.d[key]
	return found
}

// Lenght of cache is the number of items currently in the cache.
func (c *Cache[K, V]) Length() int {
	c.m.Lock()
	defer c.m.Unlock()
	return len(c.d)
}

// Drop cached item and return its last value.
func (c *Cache[K, V]) Drop(key K) (value V, ok bool) {
	c.m.Lock()
	defer c.m.Unlock()
	val, found := c.d[key]
	if found {
		c.resetTimer(val.t, 0)
		c.processTimers()
	}
	return val.Value(), found
}

// Get cached item.
//
// Since the cache can hold concrete value types, the second return
// parameter indicates whether the value was actually found in cache.
func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	c.m.Lock()
	defer c.m.Unlock()
	val, found := c.find(key)
	return val.Value(), found
}

// Put a value in cache at the given key, with the cache-default
// time-to-live.
func (c *Cache[K, V]) Put(key K, value V) {
	c.PutWithTTL(key, value, c.ttl)
}

// PutWithTTL puts a value in cache at the given key, with the given
// time-to-live.
func (c *Cache[K, V]) PutWithTTL(key K, value V, ttl time.Duration) {
	c.m.Lock()
	defer c.m.Unlock()

	val, found := c.d[key]
	val.v = value
	val.ttl = ttl
	if found {
		c.resetTimer(val.t, ttl)
	} else {
		val.t = c.addTimer(key, ttl)
	}
	c.d[key] = val
}

// GetOrPut returns the value in cache at the given key, or, if absent,
// the one returned by provider, after having put it in the cache with
// the cache-default time-to-live.
func (c *Cache[K, V]) GetOrPut(
	key K,
	provider Getter[K, V],
) (value V, ok bool) {
	return c.GetOrPutWithTTL(key, provider, c.ttl)
}

// GetOrPutWithTTL returns the value in cache at the given key, or, if
// absent, the one returned by provider, after having put it in the
// cache with the given time-to-live.
func (c *Cache[K, V]) GetOrPutWithTTL(
	key K,
	provider Getter[K, V],
	ttl time.Duration,
) (value V, ok bool) {
	c.m.Lock()
	defer c.m.Unlock()

	if val, found := c.find(key); found {
		return val.v, true
	}
	if value, ok = provider.Get(key); !ok {
		return
	}
	c.d[key] = entry[K, V]{
		t:   c.addTimer(key, ttl),
		ttl: ttl,
		v:   value,
	}
	return
}

// Touch a cached value, if present, to extend its lifetime. Returns
// false if the key has not been found in the cache.
func (c *Cache[K, T]) Touch(key K) bool {
	c.m.Lock()
	defer c.m.Unlock()
	_, found := c.find(key)
	return found
}

// Shutdown terminates the goroutine processing item expiry timers.
func (c *Cache[K, V]) Shutdown() {
	if c.IsShutDown() {
		return
	}
	close(c.done)
}

// IsShutDown returns whether item expiry timer processing is terminated.
func (c *Cache[K, V]) IsShutDown() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// A Getter can get a value for a key.
type Getter[K comparable, V any] interface {
	// Get value for given key and return if that was successful.
	Get(key K) (value V, ok bool)
}

var _ Getter[int, any] = (*GetterFunc[int, any])(nil)

// GetterFunc is a func that implements the Getter interface.
type GetterFunc[K comparable, V any] func(key K) (value V, ok bool)

func (f GetterFunc[K, V]) Get(key K) (value V, ok bool) {
	return f(key)
}

var _ Getter[int, any] = (*SimpleGetterFunc[int, any])(nil)

// SimpleGetterFunc is the simplest func implementing Getter, that
// always returns a value.
type SimpleGetterFunc[K comparable, V any] func() (value V)

func (f SimpleGetterFunc[K, V]) Get(key K) (value V, ok bool) {
	return f(), true
}

// Internals.

func (c *Cache[K, V]) loop() {
	for {
		select {
		case <-c.t.C:
			// log.Println("timer fired")
			more := true
			for more {
				c.m.Lock()
				more = c.processTimers()
				c.m.Unlock()
			}
		case <-c.done:
			return
		}
	}
}

func (c *Cache[K, V]) processTimers() (more bool) {
	if c.th.Len() == 0 {
		// log.Println("└── all timers expired, waiting indefinitely")
		c.t.Reset(indefinite)
		return
	}
	t := c.th[0]
	if now := time.Now(); t.x.After(now) {
		// log.Printf("└── expired cleared; next at %v\n", t.x)
		c.t.Reset(t.x.Sub(now))
		return
	}
	// log.Printf("├─  drop '%v' expired at %v\n", t.k, t.x)
	heap.Pop(&c.th)
	delete(c.d, t.k)
	return true
}

func (c *Cache[K, V]) find(key K) (entry[K, V], bool) {
	val, found := c.d[key]
	if found {
		c.resetTimer(val.t, val.ttl)
	}
	return val, found
}

func (c *Cache[K, V]) addTimer(key K, ttl time.Duration) *itemTimer[K] {
	t := &itemTimer[K]{
		k: key,
		x: time.Now().Add(ttl),
	}
	heap.Push(&c.th, t)
	// log.Printf("added '%v' to drop at %v\n", t.k, t.x)
	if t.i == 0 {
		// log.Println("└── this is currently the soonest")
		c.t.Reset(time.Until(t.x))
	}
	return t
}

func (c *Cache[K, V]) resetTimer(t *itemTimer[K], ttl time.Duration) {
	t.x = time.Now().Add(ttl)
	heap.Fix(&c.th, t.i)
	// log.Printf("extended '%v' to drop at %v\n", t.k, t.x)
	if t.i == 0 {
		// log.Println("└── this is currently the soonest")
		c.t.Reset(time.Until(t.x))
	}
}

type emptyChan chan struct{}

const indefinite = time.Duration(1<<63 - 1)

// entry has all the data of a stored value.
type entry[K comparable, V any] struct {
	t   *itemTimer[K] // Item expiry timer.
	ttl time.Duration // Time-to-live of the value.
	v   V             // The stored value.
}

func (e entry[K, V]) Value() V {
	return e.v
}

type itemTimer[K comparable] struct {
	i int       // Heap index.
	k K         // Key of cache entry.
	x time.Time // Expiry time.
}

type timerHeap[K comparable] []*itemTimer[K]

var _ heap.Interface = (*timerHeap[int])(nil)

// Len is the number of elements in the collection.
func (h timerHeap[_]) Len() int {
	return len(h)
}

// Less reports whether the element with index i
// must sort before the element with index j.
func (h timerHeap[_]) Less(i int, j int) bool {
	return h[i].x.Before(h[j].x)
}

// Swap swaps the elements with indexes i and j.
func (h timerHeap[_]) Swap(i int, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].i = i
	h[j].i = j
}

// Push x as element Len()
func (h *timerHeap[K]) Push(v any) {
	t := v.(*itemTimer[K])
	t.i = h.Len()
	*h = append(*h, t)
}

// Pop and return element Len() - 1.
func (h *timerHeap[_]) Pop() any {
	i := h.Len() - 1
	s := *h
	v := s[i]
	s[i] = nil
	*h = s[:i]
	return v
}
