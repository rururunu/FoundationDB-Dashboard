//go:build linux && cgo

// Package fdbstore is the FoundationDB 7.3 client behind store.Store.
// The image links libfdb_c from the 7.3.62 client package.
package fdbstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"

	"foundationdb-dashboard/internal/cluster"
	"foundationdb-dashboard/internal/config"
	"foundationdb-dashboard/internal/present"
	"foundationdb-dashboard/internal/store"
)

const (
	apiVersion      = 730
	txTimeoutMS     = 15000
	statusTTL       = 2 * time.Second
	maxPageLimit    = 200
	maxValueRead    = 1 << 20
	maxListPreview  = 64 << 10
	maxChildrenPage = 200
)

var statusKey = fdb.Key("\xff\xff/status/json")

// Client talks to one cluster. Open does not wait for the cluster to be up.
type Client struct {
	mu sync.RWMutex
	db fdb.Database

	statusMu  sync.Mutex
	statusAt  time.Time
	statusOK  bool
	statusVal cluster.Overview
	statusErr error
}

// Open selects API 730 and opens either a cluster file or a connection string.
func Open(cfg config.Config) (*Client, error) {
	if err := fdb.APIVersion(apiVersion); err != nil {
		return nil, err
	}
	var (
		db  fdb.Database
		err error
	)
	if cfg.ClusterFile != "" {
		info, statErr := os.Stat(cfg.ClusterFile)
		if statErr != nil {
			return nil, fmt.Errorf("FDB_CLUSTER_FILE: %w", statErr)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("FDB_CLUSTER_FILE %s is a directory", cfg.ClusterFile)
		}
		db, err = fdb.OpenDatabase(cfg.ClusterFile)
	} else {
		db, err = fdb.OpenWithConnectionString(cfg.ConnectionString)
	}
	if err != nil {
		return nil, err
	}
	return &Client{db: db}, nil
}

func (c *Client) database() fdb.Database {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// Use opens connection and makes it the database later calls read.
func (c *Client) Use(connection string) error {
	db, err := fdb.OpenWithConnectionString(connection)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.db = db
	c.mu.Unlock()
	c.statusMu.Lock()
	c.statusOK = false
	c.statusMu.Unlock()
	return nil
}

func (c *Client) Status(ctx context.Context) (store.Status, error) {
	ov, err := c.Overview(ctx)
	if err != nil {
		return store.Status{}, err
	}
	detail := ov.Detail
	if detail == "" && ov.Healthy {
		detail = "healthy"
	}
	return store.Status{Available: ov.Available, Detail: detail}, nil
}

// Overview reads status json at most once every two seconds.
func (c *Client) Overview(ctx context.Context) (cluster.Overview, error) {
	if err := ctx.Err(); err != nil {
		return cluster.Overview{}, err
	}
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	if c.statusOK && time.Since(c.statusAt) < statusTTL {
		return c.statusVal, c.statusErr
	}
	ov, err := c.readOverview()
	c.statusAt = time.Now()
	c.statusVal = ov
	c.statusErr = err
	c.statusOK = true
	return ov, err
}

func (c *Client) readOverview() (cluster.Overview, error) {
	raw, err := readTransact(c.database(), func(tr fdb.ReadTransaction) (any, error) {
		b, err := tr.Get(statusKey).Get()
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	})
	if err != nil {
		return c.fallback("status json unavailable"), nil
	}
	b, _ := raw.([]byte)
	if len(b) == 0 {
		return c.fallback(""), nil
	}
	return cluster.Parse(b)
}

// fallback answers from a plain read when status json cannot be read.
func (c *Client) fallback(okDetail string) cluster.Overview {
	if err := c.ping(); err != nil {
		return cluster.Overview{Available: false, Detail: err.Error()}
	}
	return cluster.Overview{Available: true, Detail: okDetail}
}

func (c *Client) ping() error {
	_, err := readTransact(c.database(), func(tr fdb.ReadTransaction) (any, error) {
		version, err := tr.GetReadVersion().Get()
		return version, err
	})
	return err
}

type scanned struct {
	key    []byte
	value  []byte
	length int
}

func (c *Client) Keys(ctx context.Context, start, end []byte, limit int) (store.Page, error) {
	if err := ctx.Err(); err != nil {
		return store.Page{}, err
	}
	if limit < 1 || limit > maxPageLimit {
		return store.Page{}, fmt.Errorf("limit must be 1-%d", maxPageLimit)
	}
	if len(end) == 0 {
		end = []byte{0xff}
	}
	if bytes.Compare(start, end) >= 0 {
		return store.Page{}, nil
	}
	begin := append([]byte(nil), start...)
	stop := append([]byte(nil), end...)
	raw, err := readTransact(c.database(), func(tr fdb.ReadTransaction) (any, error) {
		kvs, err := tr.GetRange(fdb.KeyRange{
			Begin: fdb.Key(begin),
			End:   fdb.Key(stop),
		}, fdb.RangeOptions{Limit: limit + 1}).GetSliceWithError()
		if err != nil {
			return nil, err
		}
		pairs := make([]scanned, len(kvs))
		for i, kv := range kvs {
			preview := kv.Value
			if len(preview) > maxListPreview {
				preview = preview[:maxListPreview]
			}
			pairs[i] = scanned{
				key:    append([]byte(nil), kv.Key...),
				value:  append([]byte(nil), preview...),
				length: len(kv.Value),
			}
		}
		return pairs, nil
	})
	if err != nil {
		return store.Page{}, err
	}
	pairs := raw.([]scanned)
	page := store.Page{HasMore: len(pairs) > limit}
	if page.HasMore {
		pairs = pairs[:limit]
	}
	page.Keys = make([]store.Key, len(pairs))
	for i, pair := range pairs {
		text, ok := describe(pair.key)
		summary := summarizeValue(pair.value, pair.length, false)
		page.Keys[i] = store.Key{
			Raw:         pair.key,
			TupleText:   text,
			TupleOK:     ok,
			ValueLength: pair.length,
			ValueText:   summary.Text,
			ValueKind:   summary.Kind,
		}
	}
	if page.HasMore && len(page.Keys) > 0 {
		page.Next = present.Successor(page.Keys[len(page.Keys)-1].Raw)
	}
	return page, nil
}

// Children lists the distinct next tuple elements below prefix. Each entry
// costs one small read, so a page is bounded by maxChildrenPage.
func (c *Client) Children(ctx context.Context, prefix, after []byte, limit int) (store.Children, error) {
	if err := ctx.Err(); err != nil {
		return store.Children{}, err
	}
	if limit < 1 || limit > maxChildrenPage {
		return store.Children{}, fmt.Errorf("limit must be 1-%d", maxChildrenPage)
	}
	n, ok := elementCount(prefix)
	if !ok {
		return store.Children{}, errors.New("prefix is not a tuple")
	}
	begin, end := []byte{}, []byte{0xff}
	if len(prefix) > 0 {
		begin = append(append([]byte(nil), prefix...), 0x00)
		end = append(append([]byte(nil), prefix...), 0xff)
	}
	cursor := begin
	if len(after) > 0 {
		if bytes.Compare(after, begin) < 0 || bytes.Compare(after, end) >= 0 {
			return store.Children{}, errors.New("cursor is outside the prefix")
		}
		cursor = append([]byte(nil), after...)
	}

	raw, err := readTransact(c.database(), func(tr fdb.ReadTransaction) (any, error) {
		var out store.Children
		cur := cursor
		for {
			kvs, err := tr.GetRange(fdb.KeyRange{Begin: fdb.Key(cur), End: fdb.Key(end)}, fdb.RangeOptions{Limit: 1}).GetSliceWithError()
			if err != nil {
				return nil, err
			}
			if len(kvs) == 0 {
				return out, nil
			}
			if len(out.Items) >= limit {
				out.HasMore = true
				out.Next = cur
				return out, nil
			}
			key := append([]byte(nil), kvs[0].Key...)
			child, label, isTuple := childOf(key, n)
			if !isTuple {
				text, _ := describe(key)
				if text == "" {
					text = present.HexPreview(key)
				}
				out.Items = append(out.Items, store.Child{Label: text, Prefix: key, Exact: true})
				cur = present.Successor(key)
				continue
			}
			probe, err := tr.GetRange(fdb.KeyRange{
				Begin: fdb.Key(child),
				End:   fdb.Key(append(append([]byte(nil), child...), 0xff)),
			}, fdb.RangeOptions{Limit: 2}).GetSliceWithError()
			if err != nil {
				return nil, err
			}
			exact := len(probe) > 0 && bytes.Equal(probe[0].Key, child)
			has := (exact && len(probe) > 1) || (!exact && len(probe) > 0)
			out.Items = append(out.Items, store.Child{
				Label:       label,
				Prefix:      child,
				Exact:       exact,
				HasChildren: has,
				TupleOK:     true,
			})
			cur = append(append([]byte(nil), child...), 0xff)
		}
	})
	if err != nil {
		return store.Children{}, err
	}
	return raw.(store.Children), nil
}

func (c *Client) Value(ctx context.Context, key []byte) (store.Value, error) {
	if err := ctx.Err(); err != nil {
		return store.Value{}, err
	}
	raw, err := readTransact(c.database(), func(tr fdb.ReadTransaction) (any, error) {
		val, err := tr.Get(fdb.Key(key)).Get()
		if err != nil || val == nil {
			return store.Value{}, err
		}
		return store.Value{Found: true, Data: append([]byte(nil), val...)}, nil
	})
	if err != nil {
		return store.Value{}, err
	}
	value, _ := raw.(store.Value)
	if value.Found {
		data := value.Data
		if len(data) > maxValueRead {
			data = data[:maxValueRead]
		}
		summary := summarizeValue(data, len(data), true)
		value.Text, value.Kind, value.Note = summary.Text, summary.Kind, summary.Note
	}
	return value, nil
}

// Delete removes one exact key. It does not clear a range.
func (c *Client) Delete(ctx context.Context, key []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	raw, err := c.database().Transact(func(tr fdb.Transaction) (any, error) {
		if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
			return false, err
		}
		val, err := tr.Get(fdb.Key(key)).Get()
		if err != nil {
			return false, err
		}
		if val == nil {
			return false, nil
		}
		tr.Clear(fdb.Key(key))
		return true, nil
	})
	if err != nil {
		return false, err
	}
	found, _ := raw.(bool)
	return found, nil
}

func readTransact(db fdb.Database, fn func(fdb.ReadTransaction) (any, error)) (any, error) {
	return db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
			return nil, err
		}
		return fn(tr)
	})
}
