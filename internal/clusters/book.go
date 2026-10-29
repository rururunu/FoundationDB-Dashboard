// Package clusters remembers connection strings the dashboard can open.
package clusters

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const BuiltinID = "builtin"

// Cluster is one saved connection. Builtin is the process environment and
// cannot be removed.
type Cluster struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Connection string `json:"connection"`
	Builtin    bool   `json:"builtin"`
}

// Book is the saved list and which entry is active.
type Book struct {
	mu     sync.Mutex
	path   string
	active string
	items  []Cluster
}

var connectionPattern = regexp.MustCompile(`^[A-Za-z0-9_]+:[A-Za-z0-9]+@(?:[A-Za-z0-9._-]+:\d{1,5})(?:,[A-Za-z0-9._-]+:\d{1,5})*$`)

// ValidConnection reports whether s is a single-line FoundationDB connection string.
func ValidConnection(s string) bool {
	if strings.ContainsAny(s, " \t\r\n") || !connectionPattern.MatchString(s) {
		return false
	}
	hosts := strings.Split(strings.SplitN(s, "@", 2)[1], ",")
	for _, host := range hosts {
		port := host[strings.LastIndex(host, ":")+1:]
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

// Open loads path and keeps the environment connection as the builtin entry.
// An empty path stays in memory.
func Open(path, builtinName, builtinConn string) (*Book, error) {
	book := &Book{
		path:   path,
		active: BuiltinID,
		items:  []Cluster{{ID: BuiltinID, Name: cleanName(builtinName), Connection: builtinConn, Builtin: true}},
	}
	if path == "" {
		return book, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return book, nil
		}
		return nil, err
	}
	var saved struct {
		Active string    `json:"active"`
		Items  []Cluster `json:"items"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		return book, nil
	}
	for _, item := range saved.Items {
		if item.Builtin || item.ID == BuiltinID || item.ID == "" || !ValidConnection(item.Connection) {
			continue
		}
		item.Name = cleanName(item.Name)
		item.Builtin = false
		book.items = append(book.items, item)
	}
	if saved.Active != "" && book.has(saved.Active) {
		book.active = saved.Active
	}
	return book, nil
}

// List returns the entries and the active id.
func (b *Book) List() (items []Cluster, active string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Cluster, len(b.items))
	copy(out, b.items)
	return out, b.active
}

// Get returns one entry.
func (b *Book) Get(id string) (Cluster, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, item := range b.items {
		if item.ID == id {
			return item, true
		}
	}
	return Cluster{}, false
}

// Active returns the entry the process should be using.
func (b *Book) Active() (Cluster, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, item := range b.items {
		if item.ID == b.active {
			return item, true
		}
	}
	return Cluster{}, false
}

// Add saves a new connection. It does not switch to it.
func (b *Book) Add(name, connection string) (Cluster, error) {
	name = cleanName(name)
	connection = strings.TrimSpace(connection)
	if name == "" {
		return Cluster{}, errors.New("名称不能为空")
	}
	if !ValidConnection(connection) {
		return Cluster{}, errors.New("连接串无效")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, item := range b.items {
		if item.Connection == connection {
			return Cluster{}, errors.New("这个连接已经在列表里")
		}
	}
	item := Cluster{ID: newID(), Name: name, Connection: connection}
	b.items = append(b.items, item)
	if err := b.writeLocked(); err != nil {
		b.items = b.items[:len(b.items)-1]
		return Cluster{}, err
	}
	return item, nil
}

// Activate marks id as the current cluster after a successful open.
func (b *Book) Activate(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.has(id) {
		return errors.New("集群不存在")
	}
	prev := b.active
	b.active = id
	if err := b.writeLocked(); err != nil {
		b.active = prev
		return err
	}
	return nil
}

// Remove deletes a saved cluster. The environment entry and the active entry stay.
func (b *Book) Remove(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if id == BuiltinID {
		return errors.New("不能移除启动时的集群")
	}
	if id == b.active {
		return errors.New("不能移除正在使用的集群")
	}
	prev := append([]Cluster(nil), b.items...)
	next := make([]Cluster, 0, len(prev))
	found := false
	for _, item := range prev {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return errors.New("集群不存在")
	}
	b.items = next
	if err := b.writeLocked(); err != nil {
		b.items = prev
		return err
	}
	return nil
}

func (b *Book) has(id string) bool {
	for _, item := range b.items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func (b *Book) writeLocked() error {
	if b.path == "" {
		return nil
	}
	raw, err := json.Marshal(struct {
		Active string    `json:"active"`
		Items  []Cluster `json:"items"`
	}{Active: b.active, Items: b.items})
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 40 {
		name = string([]rune(name)[:40])
	}
	return strings.ReplaceAll(name, "\n", " ")
}

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(strconv.FormatInt(int64(os.Getpid()), 16)))
	}
	return hex.EncodeToString(buf[:])
}
