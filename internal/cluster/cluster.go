// Package cluster turns the FoundationDB "status json" document into the
// numbers the overview page shows. It does not import the FoundationDB client.
package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// Overview is one reading of the cluster. Missing fields stay zero.
type Overview struct {
	Available       bool          `json:"available"`
	Detail          string        `json:"detail,omitempty"`
	Healthy         bool          `json:"healthy"`
	DataState       string        `json:"data_state"`
	Recovery        string        `json:"recovery"`
	RecoveryText    string        `json:"recovery_text"`
	Redundancy      string        `json:"redundancy"`
	StorageEngine   string        `json:"storage_engine"`
	Config          Config        `json:"config"`
	Generation      int64         `json:"generation"`
	ProtocolVersion string        `json:"protocol_version"`
	ServerVersion   string        `json:"server_version"`
	UptimeSeconds   float64       `json:"uptime_seconds"`
	Locked          bool          `json:"locked"`
	ClusterFile     string        `json:"cluster_file"`
	QuorumReachable bool          `json:"quorum_reachable"`
	Coordinators    []Coordinator `json:"coordinators"`
	Clients         int           `json:"clients"`
	Roles           Roles         `json:"roles"`
	Data            Data          `json:"data"`
	Workload        Workload      `json:"workload"`
	Latency         Latency       `json:"latency"`
	LimitedBy       string        `json:"limited_by"`
	LimitedByName   string        `json:"limited_by_name"`
	FaultTolerance  Tolerance     `json:"fault_tolerance"`
	Processes       []Process     `json:"processes"`
	Messages        []Message     `json:"messages"`
}

type Coordinator struct {
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

// Roles counts the roles that are running, not the configured numbers.
type Roles struct {
	Storage     int `json:"storage"`
	Log         int `json:"log"`
	CommitProxy int `json:"commit_proxy"`
	GrvProxy    int `json:"grv_proxy"`
	Resolver    int `json:"resolver"`
	Processes   int `json:"processes"`
}

type Data struct {
	KVBytes         int64   `json:"kv_bytes"`
	DiskBytes       int64   `json:"disk_bytes"`
	LeastFreeBytes  int64   `json:"least_free_bytes"`
	Partitions      int64   `json:"partitions"`
	MovingBytes     int64   `json:"moving_bytes"`
	MinReplicasLeft int64   `json:"min_replicas_left"`
	AvgPartition    float64 `json:"avg_partition_bytes"`
}

// Workload is per second.
type Workload struct {
	Reads        float64 `json:"reads"`
	Writes       float64 `json:"writes"`
	Started      float64 `json:"started"`
	Committed    float64 `json:"committed"`
	Conflicted   float64 `json:"conflicted"`
	ReadBytes    float64 `json:"read_bytes"`
	WrittenBytes float64 `json:"written_bytes"`
}

// Latency is in milliseconds.
type Latency struct {
	Read   float64 `json:"read_ms"`
	Commit float64 `json:"commit_ms"`
	Start  float64 `json:"start_ms"`
}

type Tolerance struct {
	Data         int64 `json:"data"`
	Availability int64 `json:"availability"`
}

type Process struct {
	ID          string   `json:"id"`
	Address     string   `json:"address"`
	Class       string   `json:"class"`
	Machine     string   `json:"machine"`
	Version     string   `json:"version"`
	Roles       []string `json:"roles"`
	CPUCores    float64  `json:"cpu_cores"`
	MemoryUsed  int64    `json:"memory_used_bytes"`
	MemoryLimit int64    `json:"memory_limit_bytes"`
	DiskFree    int64    `json:"disk_free_bytes"`
	DiskTotal   int64    `json:"disk_total_bytes"`
	DiskBusy    float64  `json:"disk_busy"`
	Uptime      float64  `json:"uptime_seconds"`
	Excluded    bool     `json:"excluded"`
	Degraded    bool     `json:"degraded"`
}

type Message struct {
	Source      string `json:"source"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Config is the editable part of cluster.configuration. Processes is how many
// processes status reported, used to reject a redundancy mode this cluster
// cannot hold.
type Config struct {
	Redundancy    string `json:"redundancy_mode"`
	StorageEngine string `json:"storage_engine"`
	CommitProxies int    `json:"commit_proxies"`
	GrvProxies    int    `json:"grv_proxies"`
	Logs          int    `json:"logs"`
	Resolvers     int    `json:"resolvers"`
	Processes     int    `json:"processes"`
}

// DiffConfig returns the special-key updates that differ from current.
func DiffConfig(cur, next Config) (map[string]string, error) {
	changes := map[string]string{}
	if next.Redundancy != cur.Redundancy {
		need, ok := redundancyProcesses[next.Redundancy]
		if !ok {
			return nil, errors.New("冗余模式无效")
		}
		procs := cur.Processes
		if procs > 0 && procs < need {
			return nil, fmt.Errorf("当前有 %d 个进程，%s 至少需要 %d 个", procs, next.Redundancy, need)
		}
		changes["redundancy_mode"] = next.Redundancy
	}
	if next.StorageEngine != cur.StorageEngine {
		if !storageEngines[next.StorageEngine] {
			return nil, errors.New("存储引擎无效")
		}
		changes["storage_engine"] = next.StorageEngine
	}
	counts := []struct {
		name string
		cur  int
		next int
	}{
		{"commit_proxies", cur.CommitProxies, next.CommitProxies},
		{"grv_proxies", cur.GrvProxies, next.GrvProxies},
		{"logs", cur.Logs, next.Logs},
		{"resolvers", cur.Resolvers, next.Resolvers},
	}
	for _, item := range counts {
		if item.next == item.cur {
			continue
		}
		if item.next < 1 || item.next > 8 {
			return nil, fmt.Errorf("%s 必须是 1 到 8", item.name)
		}
		changes[item.name] = strconv.Itoa(item.next)
	}
	return changes, nil
}

var redundancyProcesses = map[string]int{
	"single": 1, "double": 2, "triple": 3, "three_data_hall": 3,
}

var storageEngines = map[string]bool{
	"ssd-2": true, "ssd-1": true, "memory": true, "ssd-rocksdb-v1": true, "ssd-sharded-rocksdb": true,
}

// Parse reads a status json document. It fails when the document has neither
// client.database_status.available nor cluster.database_available.
func Parse(raw []byte) (Overview, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Overview{}, err
	}
	root := node(doc)
	client := root.obj("client")
	cl := root.obj("cluster")

	out := Overview{
		Coordinators: []Coordinator{},
		Processes:    []Process{},
		Messages:     []Message{},
	}
	if v, ok := client.obj("database_status").boolean("available"); ok {
		out.Available = v
	} else if v, ok := cl.boolean("database_available"); ok {
		out.Available = v
	} else {
		return Overview{}, errors.New("status json has no availability field")
	}
	if v, ok := client.obj("database_status").boolean("healthy"); ok {
		out.Healthy = v
	}

	out.DataState = cl.obj("data", "state").str("name")
	out.Recovery = cl.obj("recovery_state").str("name")
	out.RecoveryText = cl.obj("recovery_state").str("description")
	cfgNode := cl.obj("configuration")
	out.Redundancy = cfgNode.str("redundancy_mode")
	out.StorageEngine = cfgNode.str("storage_engine")
	out.Config = Config{
		Redundancy:    out.Redundancy,
		StorageEngine: out.StorageEngine,
		CommitProxies: int(cfgNode.num("commit_proxies")),
		GrvProxies:    int(cfgNode.num("grv_proxies")),
		Logs:          int(cfgNode.num("logs")),
		Resolvers:     int(cfgNode.num("resolvers")),
	}
	out.Generation = int64(cl.num("generation"))
	out.ProtocolVersion = cl.str("protocol_version")
	out.Locked, _ = cl.obj("database_lock_state").boolean("locked")
	out.ClusterFile = client.obj("cluster_file").str("path")
	out.QuorumReachable, _ = client.obj("coordinators").boolean("quorum_reachable")
	out.Clients = int(cl.obj("clients").num("count"))
	out.LimitedBy = cl.obj("qos", "performance_limited_by").str("description")
	out.LimitedByName = cl.obj("qos", "performance_limited_by").str("name")

	for _, item := range client.obj("coordinators").list("coordinators") {
		c := node(asMap(item))
		reachable, _ := c.boolean("reachable")
		out.Coordinators = append(out.Coordinators, Coordinator{Address: c.str("address"), Reachable: reachable})
	}

	data := cl.obj("data")
	out.Data = Data{
		KVBytes:         int64(data.num("total_kv_size_bytes")),
		DiskBytes:       int64(data.num("total_disk_used_bytes")),
		LeastFreeBytes:  int64(data.num("least_operating_space_bytes_storage_server")),
		Partitions:      int64(data.num("partitions_count")),
		MovingBytes:     int64(data.obj("moving_data").num("in_flight_bytes") + data.obj("moving_data").num("in_queue_bytes")),
		MinReplicasLeft: int64(data.obj("state").num("min_replicas_remaining")),
		AvgPartition:    data.num("average_partition_size_bytes"),
	}

	work := cl.obj("workload")
	out.Workload = Workload{
		Reads:        work.obj("operations", "reads").num("hz"),
		Writes:       work.obj("operations", "writes").num("hz"),
		Started:      work.obj("transactions", "started").num("hz"),
		Committed:    work.obj("transactions", "committed").num("hz"),
		Conflicted:   work.obj("transactions", "conflicted").num("hz"),
		ReadBytes:    work.obj("bytes", "read").num("hz"),
		WrittenBytes: work.obj("bytes", "written").num("hz"),
	}

	probe := cl.obj("latency_probe")
	out.Latency = Latency{
		Read:   probe.num("read_seconds") * 1000,
		Commit: probe.num("commit_seconds") * 1000,
		Start:  probe.num("transaction_start_seconds") * 1000,
	}

	out.FaultTolerance = Tolerance{
		Data:         int64(cl.obj("fault_tolerance").num("max_zone_failures_without_losing_data")),
		Availability: int64(cl.obj("fault_tolerance").num("max_zone_failures_without_losing_availability")),
	}

	versions := map[string]int{}
	for id, item := range cl.obj("processes") {
		p := node(asMap(item))
		proc := Process{
			Roles:       []string{},
			ID:          id,
			Address:     p.str("address"),
			Class:       p.str("class_type"),
			Machine:     p.str("machine_id"),
			Version:     p.str("version"),
			CPUCores:    p.obj("cpu").num("usage_cores"),
			MemoryUsed:  int64(p.obj("memory").num("used_bytes")),
			MemoryLimit: int64(p.obj("memory").num("limit_bytes")),
			DiskFree:    int64(p.obj("disk").num("free_bytes")),
			DiskTotal:   int64(p.obj("disk").num("total_bytes")),
			DiskBusy:    p.obj("disk").num("busy"),
			Uptime:      p.num("uptime_seconds"),
		}
		proc.Excluded, _ = p.boolean("excluded")
		proc.Degraded, _ = p.boolean("degraded")
		if proc.Version != "" {
			versions[proc.Version]++
		}
		for _, r := range p.list("roles") {
			role := node(asMap(r)).str("role")
			if role == "" {
				continue
			}
			proc.Roles = append(proc.Roles, role)
			switch role {
			case "storage":
				out.Roles.Storage++
			case "log":
				out.Roles.Log++
			case "commit_proxy":
				out.Roles.CommitProxy++
			case "grv_proxy":
				out.Roles.GrvProxy++
			case "resolver":
				out.Roles.Resolver++
			case "cluster_controller":
				if proc.Uptime > out.UptimeSeconds {
					out.UptimeSeconds = proc.Uptime
				}
			}
		}
		out.Processes = append(out.Processes, proc)
		out.Roles.Processes++
		for _, m := range p.list("messages") {
			mm := node(asMap(m))
			out.Messages = append(out.Messages, Message{Source: proc.Address, Name: mm.str("name"), Description: mm.str("description")})
		}
	}
	sort.Slice(out.Processes, func(i, j int) bool {
		if out.Processes[i].Address != out.Processes[j].Address {
			return out.Processes[i].Address < out.Processes[j].Address
		}
		return out.Processes[i].ID < out.Processes[j].ID
	})
	out.Config.Processes = len(out.Processes)
	best := 0
	for version, n := range versions {
		if n > best || (n == best && version > out.ServerVersion) {
			best = n
			out.ServerVersion = version
		}
	}

	for _, m := range cl.list("messages") {
		mm := node(asMap(m))
		out.Messages = append(out.Messages, Message{Source: "cluster", Name: mm.str("name"), Description: mm.str("description")})
	}
	for _, m := range client.list("messages") {
		mm := node(asMap(m))
		out.Messages = append(out.Messages, Message{Source: "client", Name: mm.str("name"), Description: mm.str("description")})
	}
	return out, nil
}

type node map[string]any

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func (n node) obj(path ...string) node {
	cur := map[string]any(n)
	for _, key := range path {
		next, _ := cur[key].(map[string]any)
		if next == nil {
			return node{}
		}
		cur = next
	}
	return node(cur)
}

func (n node) num(key string) float64 {
	v, _ := n[key].(float64)
	return v
}

func (n node) str(key string) string {
	v, _ := n[key].(string)
	return v
}

func (n node) boolean(key string) (bool, bool) {
	v, ok := n[key].(bool)
	return v, ok
}

func (n node) list(key string) []any {
	v, _ := n[key].([]any)
	return v
}
