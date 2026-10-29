# FoundationDB Dashboard

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [Русский](README.ru.md) | [日本語](README.ja.md) | [Deutsch](README.de.md) | [Français](README.fr.md)

非官方的 FoundationDB 集群网页控制台。一个 Docker 镜像内含页面和 FoundationDB **7.3.62** 客户端，数据库在镜像之外。

FoundationDB：[github.com/apple/foundationdb](https://github.com/apple/foundationdb)  
镜像：[hub.docker.com/r/runu/foundationdb-dashboard](https://hub.docker.com/r/runu/foundationdb-dashboard)

## 快速开始

```bash
docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  runu/foundationdb-dashboard:7.3.62
```

本地已有 Compose 集群时：

```bash
docker compose up -d
```

打开 [http://127.0.0.1:8090/](http://127.0.0.1:8090/)。

请把端口只发布到本机 `127.0.0.1`。进程监听 `PORT`（默认 `8080`）。

## 配置

| 变量 | 是否必需 | 说明 |
| --- | --- | --- |
| `FDB_CONNECTION_STRING` | 二选一 | 连接串，例如 `docker:docker@foundationdb:4500` |
| `FDB_CLUSTER_FILE` | 二选一 | 容器内集群文件路径；两者都设时优先用文件 |
| `PORT` | 否 | 容器内监听端口（默认 `8080`） |
| `TREND_FILE` | 否 | 24 小时读写趋势缓存路径 |
| `CLUSTERS_FILE` | 否 | 多集群目录保存路径 |
| `AUTH` | 否 | `on` / `off`（默认关闭）。开启时必须设置用户名和密码 |
| `AUTH_USER` | 开启认证时 | 登录用户名（最长 64） |
| `AUTH_PASSWORD` | 开启认证时 | 登录密码（最长 256） |

开启登录示例：

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

## 页面

### 概览

![概览](docs/imgs/overview.png)

集群是否可用、数据状态、恢复状态、冗余模式、存储引擎、协调者、进程与角色、读写负载、延迟、可容忍故障数。数据来自 `\xff\xff/status/json`，标签页可见时约每 3 秒刷新。图表由后台每 5 秒采样，保留约 24 小时。

### 数据浏览

![数据浏览](docs/imgs/data.png)

左侧目录树，中间为键表（键、值预览、类型、大小），右侧为详情。列表向下滚动会继续加载。

搜索框可直接写 tuple，不必只用十六进制：

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

支持字符串、整数、小数、`true` / `false` / `null`、`0x` 字节、`uuid(...)`、`float32(...)`、`Versionstamp(...)` 和嵌套 tuple。写前缀则列出该前缀下的键；不写则列出用户键空间。

值按 JSON、可读文本、tuple、二进制识别。4 字节和 8 字节小端整数会附说明。完整内容和可折叠十六进制在详情中。

写入能力：

- 新建、修改、重命名精确键
- 删除单个键或勾选多个精确键（需确认；服务端要求 `confirm: true`）
- 导入 / 导出、备份 / 恢复
- 盯住某个键，值变化时刷新

不提供范围删除。

### 集群配置

![集群配置](docs/imgs/configuration.png)

管理已保存的连接（添加、切换、移除）。环境自带的连接不能移除。切换集群会清空趋势缓冲。

对**当前已连接**的集群，可改冗余模式、存储引擎，以及提交代理、GRV 代理、日志、冲突检测四类角色数量。协调者、区域、加密和租户模式仍用 `fdbcli`。

## 构建与测试

```bash
make test
```

该命令跑不依赖 `libfdb_c` 的包。Docker 镜像构建会执行 `go test ./...`，包含 `internal/fdbstore`（linux + cgo）。

```bash
make build
make up
```

应用版本在镜像构建时写入（`APP_VERSION`，默认 `1.0.0`）。

## 许可说明

本项目是非官方的 FoundationDB 客户端界面。FoundationDB 本身请遵循 Apple 的许可。
