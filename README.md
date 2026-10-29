# FoundationDB Dashboard

**English** | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | [日本語](README.ja.md) | [Deutsch](README.de.md) | [Français](README.fr.md)

Unofficial Dockerized web console for FoundationDB clusters. One image ships the UI and the FoundationDB **7.3.62** client. The database stays outside the image.

FoundationDB: [github.com/apple/foundationdb](https://github.com/apple/foundationdb)  
Image: [hub.docker.com/r/runu/foundationdb-dashboard](https://hub.docker.com/r/runu/foundationdb-dashboard)

## Quick start

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  runu/foundationdb-dashboard:7.3.62
```

Against a cluster that is already on Docker Compose:

```bash
docker compose up -d
```

Open [http://127.0.0.1:8090/](http://127.0.0.1:8090/).

Publish the container port only on `127.0.0.1`. The process listens on `PORT` (default `8080`).

## Configuration

| Variable | Required | Description |
| --- | --- | --- |
| `FDB_CONNECTION_STRING` | one of these | Cluster connection string, for example `docker:docker@foundationdb:4500` |
| `FDB_CLUSTER_FILE` | one of these | Path to a cluster file inside the container. Wins when both are set |
| `PORT` | no | Listen port inside the container (default `8080`) |
| `TREND_FILE` | no | Path for the 24h read/write trend buffer |
| `CLUSTERS_FILE` | no | Path for the saved multi-cluster catalog |
| `AUTH` | no | `on` / `off` (default off). When on, `AUTH_USER` and `AUTH_PASSWORD` are required |
| `AUTH_USER` | with auth | Sign-in username (max 64 characters) |
| `AUTH_PASSWORD` | with auth | Sign-in password (max 256 characters) |

Example with sign-in:

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  -e AUTH=on \
  -e AUTH_USER=admin \
  -e AUTH_PASSWORD=secret \
  runu/foundationdb-dashboard:7.3.62
```

## Pages

### Overview

![Overview](docs/imgs/overview.png)

Cluster availability, data state, recovery, redundancy, storage engine, coordinators, processes and roles, read/write load, latency, and fault tolerance. Status comes from `\xff\xff/status/json` and refreshes every 3 seconds while the tab is visible. The chart is sampled in the background every 5 seconds and kept for 24 hours.

### Data browser

![Data browser](docs/imgs/data.png)

Directory tree on the left, key table in the middle (key, value preview, type, size), detail pane on the right. Scroll the list to load more.

Search accepts tuple text, not only hex:

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

Supported query pieces: strings, integers, floats, `true` / `false` / `null`, `0x` bytes, `uuid(...)`, `float32(...)`, `Versionstamp(...)`, and nested tuples. A prefix lists keys under that prefix; an empty query lists the user key space.

Values are classified as JSON, readable text, tuple, or binary. 4-byte and 8-byte little-endian integers get a short note. Full text and a collapsible hex dump live in the detail pane.

Writes:

- Create, edit, and rename exact keys
- Delete one key or a multi-select of exact keys (confirm dialog; server requires `confirm: true`)
- Import / export, backup / restore
- Watch a key and refresh when its value changes

There is no range delete.

### Configuration

![Configuration](docs/imgs/configuration.png)

Manage saved cluster connections (add, switch, remove). The builtin env connection cannot be removed. Switching clears the trend buffer.

For the **connected** cluster, edit redundancy mode, storage engine, and the four role counts (commit proxies, GRV proxies, logs, resolvers). Coordinators, regions, encryption, and tenant mode stay with `fdbcli`.

## Build and test

```bash
make test
```

That runs packages that do not need `libfdb_c`. The Docker image build runs `go test ./...`, including `internal/fdbstore` (linux + cgo).

```bash
make build
make up
```

App version is stamped at image build (`APP_VERSION`, default `1.0.0`).

## License note

This project is an unofficial client for FoundationDB. FoundationDB is an Apple project; see Apple’s licensing for the database itself.
