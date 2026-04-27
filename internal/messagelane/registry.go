package messagelane

import (
	"sync"
	"sync/atomic"
)

type ID uint64

const NoID ID = 0

type Registry[T any] struct {
	next    atomic.Uint64
	pending sync.Map
	count   atomic.Int64
}

type entry[T any] struct {
	ch   chan T
	once sync.Once
}

func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{}
}

func (registry *Registry[T]) Open() (ID, <-chan T, func()) {
	id := ID(registry.next.Add(1))
	item := &entry[T]{ch: make(chan T, 1)}
	registry.pending.Store(id, item)
	registry.count.Add(1)

	return id, item.ch, func() {
		registry.Cancel(id)
	}
}

func (registry *Registry[T]) Resolve(id ID, value T) bool {
	if id == NoID {
		return false
	}

	item, ok := registry.pending.LoadAndDelete(id)
	if !ok {
		return false
	}

	registry.count.Add(-1)
	item.(*entry[T]).complete(value)
	return true
}

func (registry *Registry[T]) Cancel(id ID) bool {
	if id == NoID {
		return false
	}

	item, ok := registry.pending.LoadAndDelete(id)
	if !ok {
		return false
	}

	registry.count.Add(-1)
	item.(*entry[T]).cancel()
	return true
}

func (registry *Registry[T]) Len() int {
	return int(registry.count.Load())
}

func (item *entry[T]) complete(value T) {
	item.once.Do(func() {
		item.ch <- value
		close(item.ch)
	})
}

func (item *entry[T]) cancel() {
	item.once.Do(func() {
		close(item.ch)
	})
}
