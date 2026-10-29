// Package httpapi serves the dashboard and the JSON API in front of a store.
package httpapi

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"foundationdb-dashboard/internal/auth"
	"foundationdb-dashboard/internal/cluster"
	"foundationdb-dashboard/internal/clusters"
	"foundationdb-dashboard/internal/present"
	"foundationdb-dashboard/internal/store"
	"foundationdb-dashboard/internal/trend"
	"foundationdb-dashboard/web"
)

const (
	PageDefault     = 50
	ChildrenDefault = 100
	PageMax         = 200
	maxBodyBytes    = 1 << 16
	maxImportBytes  = 8 << 20
	maxImportItems  = 2000
	maxExportKeys   = 5000
	maxValueBytes   = 1 << 20
)

// Server is the HTTP front end.
type Server struct {
	Store      store.Store
	Connection string
	Trend      *trend.Buffer
	Clusters   *clusters.Book
	Switch     func(connection string) error
	Auth       *auth.Gate
	Log        *slog.Logger
}

// Handler routes the page and the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /vendor/uplot.min.js", s.uplotJS)
	mux.HandleFunc("GET /vendor/uplot.min.css", s.uplotCSS)
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/trend", s.trend)
	mux.HandleFunc("POST /api/trend/clear", s.clearTrend)
	mux.HandleFunc("GET /api/children", s.children)
	mux.HandleFunc("GET /api/keys", s.keys)
	mux.HandleFunc("GET /api/value", s.value)
	mux.HandleFunc("GET /api/export", s.export)
	mux.HandleFunc("GET /api/backup", s.backup)
	mux.HandleFunc("POST /api/delete", s.delete)
	mux.HandleFunc("POST /api/set", s.set)
	mux.HandleFunc("POST /api/rename", s.rename)
	mux.HandleFunc("POST /api/import", s.importKeys)
	mux.HandleFunc("POST /api/configure", s.configure)
	mux.HandleFunc("GET /api/clusters", s.clusters)
	mux.HandleFunc("POST /api/clusters", s.addCluster)
	mux.HandleFunc("POST /api/clusters/use", s.useCluster)
	mux.HandleFunc("POST /api/clusters/remove", s.removeCluster)
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	if s.Auth == nil {
		return mux
	}
	return s.requireLogin(mux)
}

func (s *Server) connection() string {
	if s.Clusters != nil {
		if item, ok := s.Clusters.Active(); ok && item.Connection != "" {
			return item.Connection
		}
	}
	return s.Connection
}

func (s *Server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicWhileSignedOut(r) {
			next.ServeHTTP(w, r)
			return
		}
		token := sessionToken(r)
		if s.Auth == nil || !s.Auth.Valid(token) {
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "需要登录"})
			return
		}
		if s.Auth.Refresh(token) {
			setSessionCookie(w, token)
		}
		next.ServeHTTP(w, r)
	})
}

func publicWhileSignedOut(r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/vendor/")):
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/api/session":
		return true
	case r.Method == http.MethodPost && (r.URL.Path == "/api/login" || r.URL.Path == "/api/logout"):
		return true
	default:
		return false
	}
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"required": false, "authenticated": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"required":      true,
		"authenticated": s.Auth.Valid(sessionToken(r)),
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	token, err := s.Auth.Login(clientIP(r), req.Username, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, auth.ErrLocked) {
			status = http.StatusTooManyRequests
		}
		writeJSON(w, status, errorBody{Error: err.Error()})
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.Auth != nil {
		s.Auth.Revoke(sessionToken(r))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((12 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) uplotJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(web.UPlotJS)
}

func (s *Server) uplotCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(web.UPlotCSS)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(web.IndexHTML)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.Status(r.Context())
	if err != nil {
		s.log().Error("status", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, statusBody{
		Available:  st.Available,
		Detail:     st.Detail,
		Connection: s.connection(),
	})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.Store.Overview(r.Context())
	if err != nil {
		s.log().Error("overview", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, overviewBody{Connection: s.connection(), Overview: ov})
}

func (s *Server) clusters(w http.ResponseWriter, r *http.Request) {
	if s.Clusters == nil {
		writeJSON(w, http.StatusOK, clusterListBody{
			Active: clusters.BuiltinID,
			Clusters: []clusterItem{{
				ID: clusters.BuiltinID, Name: "cluster", Connection: s.Connection, Builtin: true, Current: true,
			}},
		})
		return
	}
	items, active := s.Clusters.List()
	out := make([]clusterItem, len(items))
	for i, item := range items {
		out[i] = clusterItem{ID: item.ID, Name: item.Name, Connection: item.Connection, Builtin: item.Builtin, Current: item.ID == active}
	}
	writeJSON(w, http.StatusOK, clusterListBody{Active: active, Clusters: out})
}

func (s *Server) addCluster(w http.ResponseWriter, r *http.Request) {
	if s.Clusters == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "没有可保存的集群列表"})
		return
	}
	var req clusterWrite
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能添加集群"})
		return
	}
	item, err := s.Clusters.Add(req.Name, req.Connection)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, clusterItem{ID: item.ID, Name: item.Name, Connection: item.Connection})
}

func (s *Server) useCluster(w http.ResponseWriter, r *http.Request) {
	if s.Clusters == nil || s.Switch == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "不能切换集群"})
		return
	}
	var req clusterWrite
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能切换集群"})
		return
	}
	item, ok := s.Clusters.Get(req.ID)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "集群不存在"})
		return
	}
	if err := s.Switch(item.Connection); err != nil {
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	if err := s.Clusters.Activate(item.ID); err != nil {
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	if s.Trend != nil {
		s.Trend.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": item.ID, "connection": item.Connection})
}

func (s *Server) removeCluster(w http.ResponseWriter, r *http.Request) {
	if s.Clusters == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "没有可保存的集群列表"})
		return
	}
	var req clusterWrite
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能移除集群"})
		return
	}
	if err := s.Clusters.Remove(req.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) trend(w http.ResponseWriter, r *http.Request) {
	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "时间位置无效"})
			return
		}
		since = n
	}
	samples := []trend.Sample{}
	if s.Trend != nil {
		samples = s.Trend.After(since)
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": samples})
}

func (s *Server) clearTrend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能删除图表数值"})
		return
	}
	if s.Trend != nil {
		s.Trend.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

func (s *Server) children(w http.ResponseWriter, r *http.Request) {
	prefix, err := present.ParseHex(r.URL.Query().Get("prefix"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "目录位置无效"})
		return
	}
	after, err := present.ParseHex(r.URL.Query().Get("after"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "继续加载的位置无效"})
		return
	}
	limit := ChildrenDefault
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 || n > PageMax {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "数量必须是 1 到 200"})
			return
		}
		limit = n
	}
	found, err := s.Store.Children(r.Context(), prefix, after, limit)
	if err != nil {
		s.log().Error("children", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	body := childrenBody{HasMore: found.HasMore, Items: make([]childBody, len(found.Items))}
	if found.HasMore {
		body.NextHex = hex.EncodeToString(found.Next)
	}
	for i, item := range found.Items {
		body.Items[i] = childBody{
			Label:       item.Label,
			PrefixHex:   hex.EncodeToString(item.Prefix),
			Exact:       item.Exact,
			HasChildren: item.HasChildren,
			TupleOK:     item.TupleOK,
		}
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) keys(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if len(query) > 4096 {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "搜索内容太长"})
		return
	}
	fuzzy := present.IsFuzzy(query)
	var start, end []byte
	var err error
	if fuzzy {
		start, end = []byte{}, []byte{0xff}
	} else {
		start, end, err = present.ParseQuery(query)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
			return
		}
	}
	if raw := r.URL.Query().Get("prefix"); raw != "" && (strings.TrimSpace(query) == "" || fuzzy) {
		packed, parseErr := present.ParseHex(raw)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "目录位置无效"})
			return
		}
		start, end = present.TupleRange(packed)
	}
	if raw := r.URL.Query().Get("start"); raw != "" {
		start, err = present.ParseHex(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "继续加载的位置无效"})
			return
		}
	}
	limit := PageDefault
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 || n > PageMax {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "每页数量必须是 1 到 200"})
			return
		}
		limit = n
	}
	var page store.Page
	if fuzzy {
		page, err = s.Store.Search(r.Context(), start, end, strings.TrimSpace(query), limit)
	} else {
		page, err = s.Store.Keys(r.Context(), start, end, limit)
	}
	if err != nil {
		s.log().Error("keys", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	body := keysBody{
		StartHex: hex.EncodeToString(start),
		Limit:    limit,
		HasMore:  page.HasMore,
		Keys:     make([]keyBody, len(page.Keys)),
	}
	if page.HasMore {
		body.NextStartHex = hex.EncodeToString(page.Next)
	}
	for i, key := range page.Keys {
		body.Keys[i] = keyBody{
			Hex:         hex.EncodeToString(key.Raw),
			TupleText:   key.TupleText,
			TupleOK:     key.TupleOK,
			ValueLength: key.ValueLength,
			ValueText:   key.ValueText,
			ValueKind:   key.ValueKind,
		}
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) value(w http.ResponseWriter, r *http.Request) {
	if _, ok := r.URL.Query()["key"]; !ok {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "缺少 key"})
		return
	}
	key, err := present.ParseHex(r.URL.Query().Get("key"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	val, err := s.Store.Value(r.Context(), key)
	if err != nil {
		s.log().Error("value", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	body := valueBody{Found: val.Found, Length: len(val.Data), Text: val.Text, Kind: val.Kind, Note: val.Note}
	if val.Found {
		data := val.Data
		if len(data) > maxValueBytes {
			data = data[:maxValueBytes]
			body.Truncated = true
		}
		body.Hex = hex.EncodeToString(data)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "请求必须是 JSON"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能删除"})
		return
	}
	key, err := present.ParseHex(req.Key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	found, err := s.Store.Delete(r.Context(), key)
	if err != nil {
		s.log().Error("delete", "err", err, "key_len", len(key))
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	s.log().Info("delete", "found", found, "key_len", len(key))
	writeJSON(w, http.StatusOK, deleteBody{Deleted: found, Found: found})
}

func (s *Server) set(w http.ResponseWriter, r *http.Request) {
	var req setRequest
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能写入"})
		return
	}
	key, err := keyFrom(req.KeyHex, req.KeyText)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	value, err := present.EncodeValue(req.Kind, req.Value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	if err := s.Store.Set(r.Context(), key, value); err != nil {
		s.log().Error("set", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": hex.EncodeToString(key), "written": true})
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能重命名"})
		return
	}
	from, err := present.ParseHex(req.FromHex)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	to, err := keyFrom(req.ToHex, req.ToText)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	if err := s.Store.Rename(r.Context(), from, to); err != nil {
		status := http.StatusBadGateway
		if err.Error() == "目标键已经存在" || err.Error() == "原来的键不存在" || err.Error() == "新旧键相同" {
			status = http.StatusConflict
		}
		s.log().Error("rename", "err", err)
		writeJSON(w, status, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": req.FromHex, "to": hex.EncodeToString(to)})
}

func (s *Server) importKeys(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "请求必须是 JSON"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能写入"})
		return
	}
	if len(req.Keys) == 0 || len(req.Keys) > maxImportItems {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "一次最多写入 2000 条"})
		return
	}
	items := make([]store.Mutation, 0, len(req.Keys))
	for _, item := range req.Keys {
		key, err := present.ParseHex(item.KeyHex)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
			return
		}
		if err := present.UserKey(key); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
			return
		}
		value, err := hex.DecodeString(item.ValueHex)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "值必须是十六进制"})
			return
		}
		items = append(items, store.Mutation{Key: key, Value: value})
	}
	n, err := s.Store.WriteMany(r.Context(), items)
	if err != nil {
		s.log().Error("import", "err", err, "written", n)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"written": n})
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	s.sendDump(w, r, "export", false)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	s.sendDump(w, r, "backup", true)
}

func (s *Server) sendDump(w http.ResponseWriter, r *http.Request, format string, whole bool) {
	start, end := []byte{}, []byte{0xff}
	query := ""
	fuzzy := false
	if !whole {
		query = r.URL.Query().Get("q")
		fuzzy = present.IsFuzzy(query)
		if !fuzzy {
			var err error
			start, end, err = present.ParseQuery(query)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
				return
			}
		}
		if raw := r.URL.Query().Get("prefix"); raw != "" && (strings.TrimSpace(query) == "" || fuzzy) {
			packed, err := present.ParseHex(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody{Error: "目录位置无效"})
				return
			}
			start, end = present.TupleRange(packed)
		}
	}
	doc := dumpBody{Format: "foundationdb-dashboard-" + format, Keys: []dumpKey{}}
	cursor := start
	for len(doc.Keys) < maxExportKeys {
		var page store.Page
		var err error
		if fuzzy {
			page, err = s.Store.Search(r.Context(), cursor, end, strings.TrimSpace(query), PageMax)
		} else {
			page, err = s.Store.Scan(r.Context(), cursor, end, PageMax)
		}
		if err != nil {
			s.log().Error(format, "err", err)
			writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
			return
		}
		for _, key := range page.Keys {
			if len(doc.Keys) >= maxExportKeys {
				doc.Truncated = true
				break
			}
			doc.Keys = append(doc.Keys, dumpKey{
				KeyHex: hex.EncodeToString(key.Raw), KeyText: key.TupleText,
				ValueHex: hex.EncodeToString(key.Value), ValueText: key.ValueText, ValueKind: key.ValueKind,
			})
		}
		if doc.Truncated || !page.HasMore {
			if page.HasMore {
				doc.Truncated = true
			}
			break
		}
		cursor = page.Next
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+format+".json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	var req configRequest
	if !decodeConfirm(w, r, maxBodyBytes, &req) {
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "需要确认后才能修改配置"})
		return
	}
	ov, err := s.Store.Overview(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	next := ov.Config
	if req.Redundancy != "" {
		next.Redundancy = req.Redundancy
	}
	if req.StorageEngine != "" {
		next.StorageEngine = req.StorageEngine
	}
	if req.CommitProxies != nil {
		next.CommitProxies = *req.CommitProxies
	}
	if req.GrvProxies != nil {
		next.GrvProxies = *req.GrvProxies
	}
	if req.Logs != nil {
		next.Logs = *req.Logs
	}
	if req.Resolvers != nil {
		next.Resolvers = *req.Resolvers
	}
	changes, err := cluster.DiffConfig(ov.Config, next)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	if err := s.Store.Configure(r.Context(), changes); err != nil {
		s.log().Error("configure", "err", err)
		writeJSON(w, http.StatusBadGateway, errorBody{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": len(changes), "changes": changes})
}

func decodeConfirm(w http.ResponseWriter, r *http.Request, limit int64, dest any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "请求必须是 JSON"})
		return false
	}
	return true
}

func keyFrom(keyHex, keyText string) ([]byte, error) {
	if strings.TrimSpace(keyHex) != "" {
		key, err := present.ParseHex(keyHex)
		if err != nil {
			return nil, err
		}
		return key, present.UserKey(key)
	}
	return present.PackExpr(keyText)
}

func (s *Server) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

type errorBody struct {
	Error string `json:"error"`
}

type statusBody struct {
	Available  bool   `json:"available"`
	Detail     string `json:"detail,omitempty"`
	Connection string `json:"connection"`
}

type keyBody struct {
	Hex         string `json:"hex"`
	TupleText   string `json:"tuple_text,omitempty"`
	TupleOK     bool   `json:"tuple_ok"`
	ValueLength int    `json:"value_length"`
	ValueText   string `json:"value_text,omitempty"`
	ValueKind   string `json:"value_kind,omitempty"`
}

type keysBody struct {
	StartHex     string    `json:"start_hex"`
	Limit        int       `json:"limit"`
	HasMore      bool      `json:"has_more"`
	NextStartHex string    `json:"next_start_hex,omitempty"`
	Keys         []keyBody `json:"keys"`
}

type valueBody struct {
	Found     bool   `json:"found"`
	Length    int    `json:"length"`
	Hex       string `json:"hex"`
	Truncated bool   `json:"truncated"`
	Text      string `json:"text,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Note      string `json:"note,omitempty"`
}

type overviewBody struct {
	Connection string `json:"connection"`
	cluster.Overview
}

type childBody struct {
	Label       string `json:"label"`
	PrefixHex   string `json:"prefix_hex"`
	Exact       bool   `json:"exact"`
	HasChildren bool   `json:"has_children"`
	TupleOK     bool   `json:"tuple_ok"`
}

type childrenBody struct {
	HasMore bool        `json:"has_more"`
	NextHex string      `json:"next_hex,omitempty"`
	Items   []childBody `json:"items"`
}

type deleteRequest struct {
	Key     string `json:"key"`
	Confirm bool   `json:"confirm"`
}

type deleteBody struct {
	Deleted bool `json:"deleted"`
	Found   bool `json:"found"`
}

type setRequest struct {
	Confirm bool   `json:"confirm"`
	KeyHex  string `json:"key_hex"`
	KeyText string `json:"key_text"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
}

type renameRequest struct {
	Confirm bool   `json:"confirm"`
	FromHex string `json:"from_hex"`
	ToHex   string `json:"to_hex"`
	ToText  string `json:"to_text"`
}

type importItem struct {
	KeyHex   string `json:"key_hex"`
	ValueHex string `json:"value_hex"`
}

type importRequest struct {
	Confirm bool         `json:"confirm"`
	Keys    []importItem `json:"keys"`
}

type dumpKey struct {
	KeyHex    string `json:"key_hex"`
	KeyText   string `json:"key_text,omitempty"`
	ValueHex  string `json:"value_hex"`
	ValueText string `json:"value_text,omitempty"`
	ValueKind string `json:"value_kind,omitempty"`
}

type dumpBody struct {
	Format    string    `json:"format"`
	Truncated bool      `json:"truncated,omitempty"`
	Keys      []dumpKey `json:"keys"`
}

type clusterItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Connection string `json:"connection"`
	Builtin    bool   `json:"builtin,omitempty"`
	Current    bool   `json:"current,omitempty"`
}

type clusterListBody struct {
	Active   string        `json:"active"`
	Clusters []clusterItem `json:"clusters"`
}

type clusterWrite struct {
	Confirm    bool   `json:"confirm"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Connection string `json:"connection"`
}

type configRequest struct {
	Confirm       bool   `json:"confirm"`
	Redundancy    string `json:"redundancy_mode"`
	StorageEngine string `json:"storage_engine"`
	CommitProxies *int   `json:"commit_proxies"`
	GrvProxies    *int   `json:"grv_proxies"`
	Logs          *int   `json:"logs"`
	Resolvers     *int   `json:"resolvers"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
