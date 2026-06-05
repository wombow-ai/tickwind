.PHONY: run build vet tidy test fmt up down

run:        ## run the API + ingest locally (memory store, no DB needed)
	go run ./cmd/server

build:      ## build the server binary
	go build -o bin/tickwind ./cmd/server

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

test:
	go test ./...

up:         ## start full stack on a server (api + postgres + redis)
	docker compose up -d --build

down:
	docker compose down
