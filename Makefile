.PHONY: test build up

test:
	go test ./internal/config ./internal/cluster ./internal/present ./internal/httpapi

build:
	docker build -t runu/foundationdb-dashboard:7.3.62 .

up:
	docker compose up -d
