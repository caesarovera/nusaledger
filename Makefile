# Makefile NusaLedger
# Dipanggil ratusan kali sehari; perintah panjang yang tidak ada di sini cenderung tidak dijalankan.
SHELL := bash
.PHONY: help test test-int race cover lint fmt migrate-up migrate-down migrate-drop up down run build tidy

DATABASE_URL ?= postgres://nusa:nusa_dev_only@127.0.0.1:5433/nusaledger?sslmode=disable
export GOFLAGS ?= -mod=readonly

help: ## daftar perintah
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

test: ## unit test cepat, tanpa Docker (< 10 detik)
	go test ./internal/...

test-int: ## integration test dengan testcontainers, butuh Docker
	go test -tags=integration -race -count=1 ./test/...

race: ## seluruh test dengan race detector
	go test -race ./...

cover: ## coverage unit test
	go test -coverprofile=coverage.out ./internal/... && go tool cover -func=coverage.out | tail -n 1

lint: ## vet + golangci-lint + govulncheck
	go vet ./...
	golangci-lint run
	govulncheck ./...

fmt: ## format kode
	gofmt -s -w .

migrate-up: ## jalankan semua migration
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down: ## batalkan satu migration terakhir
	migrate -path migrations -database "$(DATABASE_URL)" down 1

db-reset: ## HATI-HATI (dev saja): buang seluruh skema termasuk data & enum, lalu migrate up
	docker compose exec -T postgres psql -U nusa -d nusaledger -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	migrate -path migrations -database "$(DATABASE_URL)" up

up: ## nyalakan PostgreSQL lokal
	docker compose up -d postgres

down: ## matikan compose (data tetap di volume)
	docker compose down

run: ## jalankan API lokal
	go run ./cmd/api

build: ## build binary statis
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

tidy: ## rapikan go.mod
	GOFLAGS= go mod tidy
