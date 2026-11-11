package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Memory returns a store in this process's memory, keeping at most
// maxBytes of values and evicting the least recently used first. It isn't
// shared between processes: each has its own.
func Memory(maxBytes int64) Store {
	return &memory{max: maxBytes, items: map[string]*list.Element{}, lru: list.New(), counters: map[string]int64{}}
}

type memory struct {
	mu    sync.Mutex
	max   int64
	size  int64
	items map[string]*list.Element
	lru   *list.List
	// counters are Incr's, apart from the values: tag versions aren't
	// evicted for room, as losing one would make stale entries fresh.
	counters map[string]int64
}

type memItem struct {
	key     string
	value   []byte
	expires time.Time
}

func (m *memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n, ok := m.counters[key]; ok {
		return []byte(strconv.FormatInt(n, 10)), true, nil
	}
	el, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	it := el.Value.(*memItem)
	if !it.expires.IsZero() && time.Now().After(it.expires) {
		m.remove(el)
		return nil, false, nil
	}
	m.lru.MoveToFront(el)
	return it.value, true, nil
}

func (m *memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[key]; ok {
		m.remove(el)
	}
	if m.max > 0 && int64(len(value)) > m.max {
		return nil // larger than the whole cache: not kept
	}
	it := &memItem{key: key, value: value}
	if ttl > 0 {
		it.expires = time.Now().Add(ttl)
	}
	m.items[key] = m.lru.PushFront(it)
	m.size += int64(len(key) + len(value))
	for m.max > 0 && m.size > m.max {
		m.remove(m.lru.Back())
	}
	return nil
}

func (m *memory) remove(el *list.Element) {
	it := el.Value.(*memItem)
	m.lru.Remove(el)
	delete(m.items, it.key)
	m.size -= int64(len(it.key) + len(it.value))
}

func (m *memory) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		if el, ok := m.items[k]; ok {
			m.remove(el)
		}
		delete(m.counters, k)
	}
	return nil
}

func (m *memory) Incr(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key]++
	return m.counters[key], nil
}

// Disk returns a store of files in dir, one per key, which processes on
// the same machine or a shared volume share. Expired files are removed
// as they're read.
func Disk(dir string) (Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &disk{dir: dir}, nil
}

type disk struct {
	dir string
	mu  sync.Mutex // Incr's read-modify-write, within the process
}

func (d *disk) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(d.dir, h[:2], h[2:])
}

// A file is the expiry (Unix ms, 0 for none) and the value.
func (d *disk) Get(_ context.Context, key string) ([]byte, bool, error) {
	b, err := os.ReadFile(d.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(b) < 8 {
		return nil, false, nil
	}
	if exp := int64(binary.BigEndian.Uint64(b)); exp > 0 && time.Now().UnixMilli() > exp {
		_ = os.Remove(d.path(key))
		return nil, false, nil
	}
	return b[8:], true, nil
}

func (d *disk) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	p := d.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixMilli()
	}
	b := make([]byte, 8, 8+len(value))
	binary.BigEndian.PutUint64(b, uint64(exp))
	b = append(b, value...)
	// Written aside and renamed, so a reader never sees half a file.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (d *disk) Delete(_ context.Context, keys ...string) error {
	for _, k := range keys {
		if err := os.Remove(d.path(k)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (d *disk) Incr(ctx context.Context, key string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, _, err := d.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(string(b), 10, 64)
	n++
	return n, d.Set(ctx, key, []byte(strconv.FormatInt(n, 10)), 0)
}
