// Package store keeps todos in memory. A real app would use a database.
package store

import (
	"errors"
	"slices"
	"sync"
	"time"
)

// Todo is one item on a user's list.
type Todo struct {
	ID      int       `json:"id"`
	Title   string    `json:"title"`
	Done    bool      `json:"done"`
	Created time.Time `json:"created"`
}

// ErrNotFound is returned for todos that do not exist.
var ErrNotFound = errors.New("todo not found")

// Store is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	nextID int
	todos  map[string][]Todo // by user
}

// New returns an empty store.
func New() *Store { return &Store{nextID: 1, todos: map[string][]Todo{}} }

// Default is the store the app uses.
var Default = New()

// List returns a user's todos, oldest first.
func (s *Store) List(user string) []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Todo{}, s.todos[user]...)
}

// Get returns one todo.
func (s *Store) Get(user string, id int) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.todos[user] {
		if t.ID == id {
			return t, nil
		}
	}
	return Todo{}, ErrNotFound
}

// Add creates a todo.
func (s *Store) Add(user, title string) Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Todo{ID: s.nextID, Title: title, Created: time.Now().UTC()}
	s.nextID++
	s.todos[user] = append(s.todos[user], t)
	return t
}

// Toggle flips a todo's done state.
func (s *Store) Toggle(user string, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.todos[user] {
		if t.ID == id {
			s.todos[user][i].Done = !t.Done
			return nil
		}
	}
	return ErrNotFound
}

// Delete removes a todo.
func (s *Store) Delete(user string, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.todos[user]
	i := slices.IndexFunc(list, func(t Todo) bool { return t.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	s.todos[user] = slices.Delete(list, i, i+1)
	return nil
}
