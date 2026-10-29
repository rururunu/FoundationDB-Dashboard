// Package store is the database surface the HTTP API uses. The FoundationDB
// implementation lives in fdbstore so handlers can be tested without a cluster.
package store

import (
	"context"

	"foundationdb-dashboard/internal/cluster"
)

// Status is whether the cluster currently accepts reads.
type Status struct {
	Available bool
	Detail    string
}

// Key is one key from a range scan. TupleText is set only when Raw is a
// complete FoundationDB tuple. ValueText is the value as readable text, JSON,
// a tuple, or a short byte preview.
type Key struct {
	Raw         []byte
	TupleText   string
	TupleOK     bool
	ValueLength int
	ValueText   string
	ValueKind   string
	Value       []byte
}

// Page is one forward scan. Next is the inclusive start of the following page
// when HasMore is true. The scan stops before the end key.
type Page struct {
	Keys    []Key
	HasMore bool
	Next    []byte
}

// Value is the bytes stored at one exact key. Text is the full reading of
// Data, Kind is tuple, json, text, or bytes, and Note is an extra hint.
type Value struct {
	Found bool
	Data  []byte
	Text  string
	Kind  string
	Note  string
}

// Child is one entry under a tuple prefix. Prefix is the packed tuple of the
// entry. Exact means a key equals Prefix. HasChildren means keys exist below.
// TupleOK is false for a key that is not a tuple; then Prefix is the raw key.
type Child struct {
	Label       string
	Prefix      []byte
	Exact       bool
	HasChildren bool
	TupleOK     bool
}

// Children lists the distinct next tuple elements below a prefix. Next is the
// raw key to continue from when HasMore is true.
type Children struct {
	Items   []Child
	HasMore bool
	Next    []byte
}

// Mutation is one exact key and its new value.
type Mutation struct {
	Key   []byte
	Value []byte
}

// Store reads and changes individual keys. Delete, Set, and Rename never
// clear a range. Configure writes named cluster options.
type Store interface {
	Status(ctx context.Context) (Status, error)
	Overview(ctx context.Context) (cluster.Overview, error)
	Keys(ctx context.Context, start, end []byte, limit int) (Page, error)
	Search(ctx context.Context, start, end []byte, query string, limit int) (Page, error)
	Scan(ctx context.Context, start, end []byte, limit int) (Page, error)
	Children(ctx context.Context, prefix, after []byte, limit int) (Children, error)
	Value(ctx context.Context, key []byte) (Value, error)
	Delete(ctx context.Context, key []byte) (found bool, err error)
	Set(ctx context.Context, key, value []byte) error
	Rename(ctx context.Context, from, to []byte) error
	WriteMany(ctx context.Context, items []Mutation) (int, error)
	Configure(ctx context.Context, changes map[string]string) error
}
