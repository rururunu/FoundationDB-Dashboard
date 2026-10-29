# The runtime image contains this service and the FoundationDB 7.3.62 client.
# The database server stays outside the image.
FROM debian:bookworm-slim AS deb

ARG FDB_VERSION=7.3.62
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl -fsSL -o /fdb-clients.deb \
        "https://github.com/apple/foundationdb/releases/download/${FDB_VERSION}/foundationdb-clients_${FDB_VERSION}-1_amd64.deb" \
    && rm -rf /var/lib/apt/lists/*

FROM golang:1.23-bookworm AS build

COPY --from=deb /fdb-clients.deb /tmp/fdb-clients.deb
RUN apt-get update \
    && apt-get install -y --no-install-recommends /tmp/fdb-clients.deb \
    && ldconfig \
    && rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1 \
    CGO_LDFLAGS="-lfdb_c -lm -lpthread -lrt"

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./...
RUN go build -trimpath -ldflags="-s -w" -o /out/foundationdb-dashboard ./cmd/foundationdb-dashboard

FROM debian:bookworm-slim

COPY --from=deb /fdb-clients.deb /tmp/fdb-clients.deb
RUN apt-get update \
    && apt-get install -y --no-install-recommends /tmp/fdb-clients.deb \
    && ldconfig \
    && rm /tmp/fdb-clients.deb \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --create-home --shell /usr/sbin/nologin dashboard

COPY --from=build /out/foundationdb-dashboard /usr/local/bin/foundationdb-dashboard
RUN mkdir -p /var/lib/foundationdb-dashboard \
    && chown dashboard:dashboard /var/lib/foundationdb-dashboard
USER dashboard
ENV PORT=8080 \
    TREND_FILE=/var/lib/foundationdb-dashboard/trend.json
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/foundationdb-dashboard"]
