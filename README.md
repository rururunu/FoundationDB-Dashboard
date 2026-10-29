# FoundationDB dashboard

一个镜像，里面是这个页面和 FoundationDB 7.3.62 客户端。数据库在镜像外面，用 `FDB_CLUSTER_FILE` 或 `FDB_CONNECTION_STRING` 接入。

```bash
docker build -t foundationdb-dashboard:7.3.62 .

docker run --rm \
  -p 127.0.0.1:8090:8080 \
  --network local-foundationdb_default \
  -e FDB_CONNECTION_STRING=docker:docker@foundationdb:4500 \
  foundationdb-dashboard:7.3.62
```

已经在跑的本地集群：

```bash
docker compose up -d --build
```

打开 http://127.0.0.1:8090/

进程监听 `PORT`（默认 8080）。把该端口发布到本机 `127.0.0.1`。`FDB_CLUSTER_FILE` 是容器内的集群文件路径；两者同时设置时使用集群文件。

## 页面

- **概览**：集群是否可用、数据状态、恢复状态、冗余模式、存储引擎、协调者与仲裁、进程与角色、读写负载、延迟、可容忍故障数。数据来自 `\xff\xff/status/json`，每 3 秒刷新一次。
- **数据浏览**：左边是目录树，中间是键、值、类型、大小，右边是选中键的完整内容。列表向下滚动时自动加载更多。

搜索框直接写 tuple，不用十六进制：

```text
"M"
("M", uuid(3c6047fa-4f4e-466f-b349-49b72f5d6260), "l")
```

支持字符串、整数、小数、`true` / `false` / `null`、`0x` 字节、`uuid(...)`、`float32(...)`、`Versionstamp(...)` 和嵌套 tuple。写一个前缀就列出这个前缀下的所有键，不写就列出整个用户键空间。

值按 JSON、可读文本、tuple、二进制的顺序识别。8 字节小端整数会附一行说明。更多内容在右侧详情里，十六进制可折叠查看。

删除只针对单个键，点击后要在对话框里确认。服务端要求请求里带 `confirm: true`，不提供范围删除。

## 测试

不依赖客户端的测试：

```bash
make test
```

`internal/fdbstore` 通过 cgo 链接 `libfdb_c`，在镜像构建里随 `go test ./...` 一起跑。
