package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"time"

	"foundationdb-dashboard/internal/cluster"
	"foundationdb-dashboard/internal/clusters"
	"foundationdb-dashboard/internal/store"
	"foundationdb-dashboard/internal/trend"
	"foundationdb-dashboard/web"
)

type fakeStore struct {
	status   store.Status
	overview cluster.Overview
	children store.Children
	page     store.Page
	value    store.Value
	err      error
	seen     [][]byte
	ends     [][]byte
}

func (f *fakeStore) Status(context.Context) (store.Status, error) {
	return f.status, f.err
}

func (f *fakeStore) Overview(context.Context) (cluster.Overview, error) {
	return f.overview, f.err
}

func (f *fakeStore) Children(_ context.Context, prefix, after []byte, _ int) (store.Children, error) {
	f.seen = append(f.seen, append([]byte(nil), prefix...), append([]byte(nil), after...))
	return f.children, f.err
}

func (f *fakeStore) Keys(_ context.Context, start, end []byte, limit int) (store.Page, error) {
	if f.err != nil {
		return store.Page{}, f.err
	}
	if limit != 2 {
		return store.Page{}, errLimit
	}
	f.seen = append(f.seen, append([]byte(nil), start...))
	f.ends = append(f.ends, append([]byte(nil), end...))
	return f.page, nil
}

func (f *fakeStore) Value(context.Context, []byte) (store.Value, error) {
	return f.value, f.err
}

func (f *fakeStore) Delete(_ context.Context, key []byte) (bool, error) {
	f.seen = append(f.seen, append([]byte(nil), key...))
	return f.value.Found, f.err
}

func (f *fakeStore) Search(_ context.Context, start, end []byte, query string, limit int) (store.Page, error) {
	if f.err != nil {
		return store.Page{}, f.err
	}
	if limit != 2 {
		return store.Page{}, errLimit
	}
	f.seen = append(f.seen, []byte(query))
	f.ends = append(f.ends, append([]byte(nil), end...))
	_ = start
	return f.page, nil
}

func (f *fakeStore) Scan(_ context.Context, start, end []byte, limit int) (store.Page, error) {
	return f.Keys(context.Background(), start, end, limit)
}

func (f *fakeStore) Set(_ context.Context, key, value []byte) error {
	f.seen = append(f.seen, append([]byte(nil), key...), append([]byte(nil), value...))
	return f.err
}

func (f *fakeStore) Rename(_ context.Context, from, to []byte) error {
	f.seen = append(f.seen, append([]byte(nil), from...), append([]byte(nil), to...))
	return f.err
}

func (f *fakeStore) WriteMany(_ context.Context, items []store.Mutation) (int, error) {
	return len(items), f.err
}

func (f *fakeStore) Configure(_ context.Context, changes map[string]string) error {
	f.seen = append(f.seen, []byte(changes["commit_proxies"]))
	return f.err
}

var errLimit = errString("limit")

type errString string

func (e errString) Error() string { return string(e) }

func TestUPlotAssets(t *testing.T) {
	h := (&Server{Store: &fakeStore{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	for _, path := range []string{"/vendor/uplot.min.js", "/vendor/uplot.min.css"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() < 100 {
			t.Fatalf("%s %d len %d", path, rec.Code, rec.Body.Len())
		}
	}
}

func TestPageRequiresDeleteConfirmation(t *testing.T) {
	if !bytes.Contains(web.IndexHTML, []byte("确认删除")) {
		t.Fatal("page is missing the delete confirmation")
	}
}

func TestStatusAndKeys(t *testing.T) {
	fake := &fakeStore{
		status: store.Status{Available: true, Detail: "healthy"},
		page: store.Page{
			Keys: []store.Key{{
				Raw:       []byte{0x02, 'a', 0x00},
				TupleText: `("a",)`,
				TupleOK:   true,
			}},
			HasMore: true,
			Next:    []byte{0x02, 'a', 0x00, 0x00},
		},
	}
	srv := httptest.NewServer((&Server{
		Store:      fake,
		Connection: "docker:docker@foundationdb:4500",
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler())
	defer srv.Close()

	status := getJSON(t, srv.URL+"/api/status")
	if status["available"] != true {
		t.Fatalf("status %#v", status)
	}
	if status["connection"] != "docker:docker@foundationdb:4500" {
		t.Fatalf("connection %#v", status["connection"])
	}

	keys := getJSON(t, srv.URL+"/api/keys?limit=2&start=02")
	rows, _ := keys["keys"].([]any)
	if len(rows) != 1 || keys["has_more"] != true {
		t.Fatalf("keys %#v", keys)
	}
	row := rows[0].(map[string]any)
	if row["tuple_ok"] != true || row["tuple_text"] != `("a",)` {
		t.Fatalf("row %#v", row)
	}
	if keys["next_start_hex"] != "02610000" {
		t.Fatalf("next %v", keys["next_start_hex"])
	}
	if len(fake.seen) != 1 || !bytes.Equal(fake.seen[0], []byte{0x02}) {
		t.Fatalf("start %#v", fake.seen)
	}

	filtered := getJSON(t, srv.URL+"/api/keys?limit=2&q=%22a%22")
	if filtered["has_more"] != true {
		t.Fatalf("filtered %#v", filtered)
	}
	if len(fake.seen) != 2 || !bytes.Equal(fake.seen[1], []byte{0x02, 'a', 0x00}) {
		t.Fatalf("query start %#v", fake.seen)
	}

	bad, err := http.Get(srv.URL + "/api/keys?limit=2&q=%28%22a%22")
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad query %d", bad.StatusCode)
	}

	res, err := http.Get(srv.URL + "/api/keys?limit=999")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("limit status %d", res.StatusCode)
	}
}

func TestOverviewAndChildren(t *testing.T) {
	fake := &fakeStore{
		overview: cluster.Overview{Available: true, Redundancy: "single", Clients: 3},
		children: store.Children{
			Items:   []store.Child{{Label: `"M"`, Prefix: []byte{0x02, 'M', 0x00}, HasChildren: true, TupleOK: true}},
			HasMore: true,
			Next:    []byte{0x02, 'N'},
		},
	}
	srv := httptest.NewServer((&Server{
		Store:      fake,
		Connection: "docker:docker@foundationdb:4500",
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler())
	defer srv.Close()

	ov := getJSON(t, srv.URL+"/api/overview")
	if ov["available"] != true || ov["redundancy"] != "single" || ov["clients"] != float64(3) {
		t.Fatalf("overview %#v", ov)
	}
	if ov["connection"] != "docker:docker@foundationdb:4500" {
		t.Fatalf("connection %#v", ov["connection"])
	}

	kids := getJSON(t, srv.URL+"/api/children?prefix=024d00&after=02")
	items, _ := kids["items"].([]any)
	if len(items) != 1 || kids["has_more"] != true || kids["next_hex"] != "024e" {
		t.Fatalf("children %#v", kids)
	}
	item := items[0].(map[string]any)
	if item["label"] != `"M"` || item["prefix_hex"] != "024d00" || item["has_children"] != true {
		t.Fatalf("item %#v", item)
	}
	if len(fake.seen) != 2 || !bytes.Equal(fake.seen[0], []byte{0x02, 'M', 0x00}) || !bytes.Equal(fake.seen[1], []byte{0x02}) {
		t.Fatalf("children args %#v", fake.seen)
	}

	res, err := http.Get(srv.URL + "/api/children?prefix=zz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad prefix %d", res.StatusCode)
	}
}

func TestKeysPrefixRange(t *testing.T) {
	fake := &fakeStore{page: store.Page{}}
	srv := httptest.NewServer((&Server{Store: fake, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler())
	defer srv.Close()

	getJSON(t, srv.URL+"/api/keys?limit=2&prefix=024d00")
	if len(fake.seen) != 1 || !bytes.Equal(fake.seen[0], []byte{0x02, 'M', 0x00}) {
		t.Fatalf("start %#v", fake.seen)
	}
	if !bytes.Equal(fake.ends[0], []byte{0x02, 'M', 0x00, 0xff}) {
		t.Fatalf("end %x", fake.ends[0])
	}

	getJSON(t, srv.URL+"/api/keys?limit=2&q=%22a%22&prefix=024d00")
	if !bytes.Equal(fake.seen[1], []byte{0x02, 'a', 0x00}) {
		t.Fatalf("search must win over the folder: %x", fake.seen[1])
	}
}

func TestValueAndDelete(t *testing.T) {
	fake := &fakeStore{value: store.Value{Found: true, Data: []byte{0x0a, 0xff}}}
	h := (&Server{Store: fake, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/value?key=0a", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("value %d %s", rec.Code, rec.Body.String())
	}
	var value valueBody
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Found || value.Length != 2 || value.Hex != "0aff" {
		t.Fatalf("%+v", value)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/delete", bytes.NewBufferString(`{"key":"0a"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed delete %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.seen) != 0 {
		t.Fatal("delete ran without confirmation")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/delete", bytes.NewBufferString(`{"key":"0a","confirm":true}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.seen) != 1 || !bytes.Equal(fake.seen[0], []byte{0x0a}) {
		t.Fatalf("deleted %#v", fake.seen)
	}
}

func TestTrendClearNeedsConfirm(t *testing.T) {
	buf := trend.New("")
	buf.Add(time.Now(), 4, 5)
	h := (&Server{Store: &fakeStore{}, Trend: buf, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trend/clear", bytes.NewBufferString(`{"confirm":false}`)))
	if rec.Code != http.StatusBadRequest || len(buf.After(0)) != 1 {
		t.Fatalf("unconfirmed %d left %d", rec.Code, len(buf.After(0)))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trend?since=0", nil))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"r":4`)) {
		t.Fatalf("trend %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trend/clear", bytes.NewBufferString(`{"confirm":true}`)))
	if rec.Code != http.StatusOK || len(buf.After(0)) != 0 {
		t.Fatalf("clear %d left %d", rec.Code, len(buf.After(0)))
	}
}

func TestFuzzySearchAndSet(t *testing.T) {
	fake := &fakeStore{page: store.Page{Keys: []store.Key{{
		Raw: []byte{0x02}, TupleText: `("demo", "bytes", 3)`, TupleOK: true, ValueText: "ff", ValueKind: "bytes",
	}}}}
	h := (&Server{Store: fake, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/keys?limit=2&q=bytes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fuzzy %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.seen) != 1 || string(fake.seen[0]) != "bytes" {
		t.Fatalf("query %#v", fake.seen)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/set", bytes.NewBufferString(`{"key_text":"(\"a\",)","kind":"text","value":"hi"}`)))
	if rec.Code != http.StatusBadRequest || len(fake.seen) != 1 {
		t.Fatalf("unconfirmed set %d seen %d", rec.Code, len(fake.seen))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/set", bytes.NewBufferString(`{"confirm":true,"key_text":"(\"a\",)","kind":"text","value":"hi"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("set %d %s", rec.Code, rec.Body.String())
	}
}

func TestClusterAddDoesNotSwitchUntilUse(t *testing.T) {
	book, err := clusters.Open(filepath.Join(t.TempDir(), "clusters.json"), "docker", "docker:docker@foundationdb:4500")
	if err != nil {
		t.Fatal(err)
	}
	var opened []string
	h := (&Server{
		Store:      &fakeStore{},
		Connection: "docker:docker@foundationdb:4500",
		Clusters:   book,
		Switch: func(connection string) error {
			opened = append(opened, connection)
			return nil
		},
		Trend: trend.New(""),
	}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/clusters", bytes.NewBufferString(`{"confirm":true,"name":"lab","connection":"lab:lab@127.0.0.1:4501"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("add %d %s", rec.Code, rec.Body.String())
	}
	if len(opened) != 0 {
		t.Fatalf("add opened %v", opened)
	}
	var added clusterItem
	if err := json.Unmarshal(rec.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/clusters/use", bytes.NewBufferString(`{"confirm":true,"id":"`+added.ID+`"}`)))
	if rec.Code != http.StatusOK || len(opened) != 1 || opened[0] != "lab:lab@127.0.0.1:4501" {
		t.Fatalf("use %d %s opened %v", rec.Code, rec.Body.String(), opened)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/clusters/remove", bytes.NewBufferString(`{"confirm":true,"id":"`+added.ID+`"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("remove active %d", rec.Code)
	}
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %d %s", url, res.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
