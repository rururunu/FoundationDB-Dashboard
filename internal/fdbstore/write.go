//go:build linux && cgo

package fdbstore

import (
	"bytes"
	"context"
	"errors"

	"github.com/apple/foundationdb/bindings/go/src/fdb"

	"foundationdb-dashboard/internal/present"
	"foundationdb-dashboard/internal/store"
)

const (
	searchBatch  = 80
	maxExamined  = 4000
	writeBatch   = 40
	configPrefix = "\xff\xff/configuration/"
)

func (c *Client) scan(ctx context.Context, start, end []byte, limit int, full bool) (store.Page, error) {
	if err := ctx.Err(); err != nil {
		return store.Page{}, err
	}
	if limit < 1 || limit > maxPageLimit {
		return store.Page{}, errors.New("limit is out of range")
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
		kvs, err := tr.GetRange(fdb.KeyRange{Begin: fdb.Key(begin), End: fdb.Key(stop)}, fdb.RangeOptions{Limit: limit + 1}).GetSliceWithError()
		if err != nil {
			return nil, err
		}
		pairs := make([]scanned, len(kvs))
		for i, kv := range kvs {
			value := kv.Value
			if !full && len(value) > maxListPreview {
				value = value[:maxListPreview]
			}
			if full && len(value) > maxValueRead {
				value = value[:maxValueRead]
			}
			pairs[i] = scanned{key: append([]byte(nil), kv.Key...), value: append([]byte(nil), value...), length: len(kv.Value)}
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
		summary := summarizeValue(pair.value, pair.length, full)
		page.Keys[i] = store.Key{
			Raw: pair.key, TupleText: text, TupleOK: ok,
			ValueLength: pair.length, ValueText: summary.Text, ValueKind: summary.Kind, Value: pair.value,
		}
	}
	if page.HasMore && len(page.Keys) > 0 {
		page.Next = present.Successor(page.Keys[len(page.Keys)-1].Raw)
	}
	return page, nil
}

// Scan reads exact keys and their values for export and backup.
func (c *Client) Scan(ctx context.Context, start, end []byte, limit int) (store.Page, error) {
	return c.scan(ctx, start, end, limit, true)
}

// Search keeps keys whose displayed text or value contains query.
func (c *Client) Search(ctx context.Context, start, end []byte, query string, limit int) (store.Page, error) {
	if err := ctx.Err(); err != nil {
		return store.Page{}, err
	}
	if limit < 1 || limit > maxPageLimit {
		return store.Page{}, errors.New("limit is out of range")
	}
	if len(end) == 0 {
		end = []byte{0xff}
	}
	cursor := append([]byte(nil), start...)
	var matched []store.Key
	examined := 0
	for examined < maxExamined && len(matched) <= limit {
		page, err := c.scan(ctx, cursor, end, searchBatch, false)
		if err != nil {
			return store.Page{}, err
		}
		if len(page.Keys) == 0 {
			return store.Page{Keys: matched}, nil
		}
		for _, key := range page.Keys {
			examined++
			label := key.TupleText
			if !key.TupleOK {
				label = present.HexPreview(key.Raw)
			}
			if present.Matches(label, key.ValueText, query) {
				matched = append(matched, key)
				if len(matched) > limit {
					out := store.Page{Keys: matched[:limit], HasMore: true, Next: present.Successor(matched[limit-1].Raw)}
					return out, nil
				}
			}
			if examined >= maxExamined {
				next := present.Successor(key.Raw)
				hasMore := page.HasMore || bytes.Compare(next, end) < 0
				return store.Page{Keys: matched, HasMore: hasMore, Next: next}, nil
			}
		}
		if !page.HasMore {
			return store.Page{Keys: matched}, nil
		}
		cursor = page.Next
	}
	next := cursor
	if len(matched) > 0 {
		next = present.Successor(matched[len(matched)-1].Raw)
	}
	return store.Page{Keys: matched, HasMore: true, Next: next}, nil
}

// Set writes one exact key.
func (c *Client) Set(ctx context.Context, key, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := present.UserKey(key); err != nil {
		return err
	}
	_, err := c.database().Transact(func(tr fdb.Transaction) (any, error) {
		if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
			return nil, err
		}
		tr.Set(fdb.Key(key), value)
		return nil, nil
	})
	return err
}

// Rename copies one exact key onto another and clears the old key.
func (c *Client) Rename(ctx context.Context, from, to []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := present.UserKey(from); err != nil {
		return err
	}
	if err := present.UserKey(to); err != nil {
		return err
	}
	if bytes.Equal(from, to) {
		return errors.New("新旧键相同")
	}
	_, err := c.database().Transact(func(tr fdb.Transaction) (any, error) {
		if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
			return nil, err
		}
		old, err := tr.Get(fdb.Key(from)).Get()
		if err != nil {
			return nil, err
		}
		if old == nil {
			return nil, errors.New("原来的键不存在")
		}
		exist, err := tr.Get(fdb.Key(to)).Get()
		if err != nil {
			return nil, err
		}
		if exist != nil {
			return nil, errors.New("目标键已经存在")
		}
		tr.Set(fdb.Key(to), old)
		tr.Clear(fdb.Key(from))
		return nil, nil
	})
	return err
}

// WriteMany sets exact keys in small transactions. It does not clear a range.
func (c *Client) WriteMany(ctx context.Context, items []store.Mutation) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	written := 0
	for i := 0; i < len(items); i += writeBatch {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		end := i + writeBatch
		if end > len(items) {
			end = len(items)
		}
		chunk := items[i:end]
		_, err := c.database().Transact(func(tr fdb.Transaction) (any, error) {
			if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
				return nil, err
			}
			for _, item := range chunk {
				if err := present.UserKey(item.Key); err != nil {
					return nil, err
				}
				tr.Set(fdb.Key(item.Key), item.Value)
			}
			return nil, nil
		})
		if err != nil {
			return written, err
		}
		written += len(chunk)
	}
	return written, nil
}

// Configure writes changed options through the configuration special-key space.
func (c *Client) Configure(ctx context.Context, changes map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	_, err := c.database().Transact(func(tr fdb.Transaction) (any, error) {
		if err := tr.Options().SetTimeout(txTimeoutMS); err != nil {
			return nil, err
		}
		if err := tr.Options().SetSpecialKeySpaceEnableWrites(); err != nil {
			return nil, err
		}
		for name, value := range changes {
			tr.Set(fdb.Key(configPrefix+name), []byte(value))
		}
		return nil, nil
	})
	return err
}
