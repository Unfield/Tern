// Package store keeps todos in memory. Swap it for a database when you
// need one; the API stays the same.
package store

import (
	"errors"
	"slices"
	"sync"
	"time"
)

// Todo is one item.
type Todo struct {
	ID      int64     `json:"id"`
	Title   string    `json:"title"`
	Done    bool      `json:"done"`
	Created time.Time `json:"created"`
}

// ErrNotFound is returned for unknown IDs.
var ErrNotFound = errors.New("not found")

var (
	mu    sync.Mutex
	next  int64 = 1
	todos []Todo
)

// List returns the todos, optionally only done or open ones.
func List(done *bool) []Todo {
	mu.Lock()
	defer mu.Unlock()
	out := []Todo{}
	for _, t := range todos {
		if done == nil || t.Done == *done {
			out = append(out, t)
		}
	}
	return out
}

// Add creates a todo.
func Add(title string) Todo {
	mu.Lock()
	defer mu.Unlock()
	t := Todo{ID: next, Title: title, Created: time.Now()}
	next++
	todos = append(todos, t)
	return t
}

// Get returns one todo.
func Get(id int64) (Todo, error) {
	mu.Lock()
	defer mu.Unlock()
	if i := slices.IndexFunc(todos, func(t Todo) bool { return t.ID == id }); i >= 0 {
		return todos[i], nil
	}
	return Todo{}, ErrNotFound
}

// Update changes a todo with fn.
func Update(id int64, fn func(*Todo)) (Todo, error) {
	mu.Lock()
	defer mu.Unlock()
	i := slices.IndexFunc(todos, func(t Todo) bool { return t.ID == id })
	if i < 0 {
		return Todo{}, ErrNotFound
	}
	fn(&todos[i])
	return todos[i], nil
}

// Delete removes a todo.
func Delete(id int64) error {
	mu.Lock()
	defer mu.Unlock()
	i := slices.IndexFunc(todos, func(t Todo) bool { return t.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	todos = slices.Delete(todos, i, i+1)
	return nil
}
