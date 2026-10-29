package cluster

import "testing"

const sample = `{
  "client": {
    "cluster_file": {"path": "/var/fdb/fdb.cluster", "up_to_date": true},
    "coordinators": {"coordinators": [{"address": "172.20.0.2:4500", "reachable": true}], "quorum_reachable": true},
    "database_status": {"available": true, "healthy": true},
    "messages": []
  },
  "cluster": {
    "clients": {"count": 61},
    "configuration": {"redundancy_mode": "single", "storage_engine": "ssd-2"},
    "data": {
      "state": {"name": "healthy", "min_replicas_remaining": 1},
      "total_kv_size_bytes": 18696000,
      "total_disk_used_bytes": 209805312,
      "least_operating_space_bytes_storage_server": 774471278010,
      "partitions_count": 1,
      "moving_data": {"in_flight_bytes": 5, "in_queue_bytes": 7}
    },
    "database_lock_state": {"locked": false},
    "fault_tolerance": {"max_zone_failures_without_losing_availability": 0, "max_zone_failures_without_losing_data": 0},
    "generation": 36,
    "latency_probe": {"read_seconds": 0.0000143, "commit_seconds": 0.0045, "transaction_start_seconds": 0.0000667},
    "messages": [{"name": "status_incomplete", "description": "Unable to retrieve all status information."}],
    "processes": {
      "p1": {
        "address": "172.20.0.2:4500", "class_type": "unset", "machine_id": "m1", "version": "7.3.62",
        "uptime_seconds": 14575.1, "excluded": false, "degraded": false,
        "cpu": {"usage_cores": 0.0125}, "memory": {"used_bytes": 254058496, "limit_bytes": 8589934592},
        "disk": {"free_bytes": 825776771072, "total_bytes": 1026108821504, "busy": 0.0088},
        "roles": [{"role": "cluster_controller"}, {"role": "storage"}, {"role": "log"}, {"role": "commit_proxy"}, {"role": "grv_proxy"}, {"role": "resolver"}, {"role": "coordinator"}]
      }
    },
    "protocol_version": "fdb00b073000000",
    "qos": {"performance_limited_by": {"description": "The database is not being saturated by the workload.", "name": "workload"}},
    "recovery_state": {"name": "fully_recovered", "description": "Recovery complete."},
    "workload": {
      "bytes": {"read": {"hz": 1385.19}, "written": {"hz": 14.4}},
      "operations": {"reads": {"hz": 12.8}, "writes": {"hz": 0.4}},
      "transactions": {"started": {"hz": 7.6}, "committed": {"hz": 0.4}, "conflicted": {"hz": 0}}
    }
  }
}`

func TestParseOverview(t *testing.T) {
	o, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if !o.Available || !o.Healthy || o.DataState != "healthy" || o.Recovery != "fully_recovered" {
		t.Fatalf("state %+v", o)
	}
	if o.Redundancy != "single" || o.StorageEngine != "ssd-2" || o.Generation != 36 || o.ServerVersion != "7.3.62" {
		t.Fatalf("config %+v", o)
	}
	if o.Roles.Storage != 1 || o.Roles.Log != 1 || o.Roles.CommitProxy != 1 || o.Roles.GrvProxy != 1 || o.Roles.Processes != 1 {
		t.Fatalf("roles %+v", o.Roles)
	}
	if len(o.Coordinators) != 1 || !o.Coordinators[0].Reachable || !o.QuorumReachable {
		t.Fatalf("coordinators %+v", o.Coordinators)
	}
	if o.Clients != 61 || o.ClusterFile != "/var/fdb/fdb.cluster" {
		t.Fatalf("clients %d file %q", o.Clients, o.ClusterFile)
	}
	if o.Data.KVBytes != 18696000 || o.Data.DiskBytes != 209805312 || o.Data.MovingBytes != 12 {
		t.Fatalf("data %+v", o.Data)
	}
	if o.Workload.Reads != 12.8 || o.Workload.ReadBytes != 1385.19 || o.Workload.Started != 7.6 {
		t.Fatalf("workload %+v", o.Workload)
	}
	if o.Latency.Commit < 4.49 || o.Latency.Commit > 4.51 {
		t.Fatalf("latency %+v", o.Latency)
	}
	if o.UptimeSeconds != 14575.1 {
		t.Fatalf("uptime %v", o.UptimeSeconds)
	}
	if len(o.Processes) != 1 || len(o.Processes[0].Roles) != 7 || o.Processes[0].MemoryUsed != 254058496 {
		t.Fatalf("processes %+v", o.Processes)
	}
	if len(o.Messages) != 1 || o.Messages[0].Name != "status_incomplete" || o.Messages[0].Source != "cluster" {
		t.Fatalf("messages %+v", o.Messages)
	}
	if o.LimitedBy == "" || o.LimitedByName != "workload" {
		t.Fatalf("limited_by %q name %q", o.LimitedBy, o.LimitedByName)
	}
}

func TestParseEmptyListsAreArrays(t *testing.T) {
	o, err := Parse([]byte(`{"cluster":{"database_available":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if o.Coordinators == nil || o.Processes == nil || o.Messages == nil {
		t.Fatalf("lists must be empty, not nil: %+v", o)
	}
}

func TestParseAvailability(t *testing.T) {
	down, err := Parse([]byte(`{"cluster":{"database_available":false}}`))
	if err != nil || down.Available {
		t.Fatalf("down=%+v err=%v", down, err)
	}
	if _, err := Parse([]byte(`{}`)); err == nil {
		t.Fatal("missing availability must fail")
	}
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("bad json must fail")
	}
}
