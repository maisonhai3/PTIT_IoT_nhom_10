# Lệnh tắt cho các việc hay làm. `make help` để xem danh sách.
.DEFAULT_GOAL := help
.PHONY: help demo test test-backend test-frontend test-firmware e2e build up down

help: ## Liệt kê các lệnh
	@grep -E '^[a-z0-9-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-14s %s\n", $$1, $$2}'

demo: ## Chạy cả hệ thống không cần phần cứng: mở http://127.0.0.1:8080
	cd backend && go run ./cmd/demo

test: test-backend test-frontend test-firmware ## Chạy mọi test (firmware cần PlatformIO)

test-backend: ## Go: gofmt, vet, test có -race (gồm end-to-end qua broker nhúng)
	cd backend && test -z "$$(gofmt -l .)" || (gofmt -l . && echo "gofmt: cần định dạng lại các file trên" && exit 1)
	cd backend && go vet ./... && go test -race -count=1 ./...

test-frontend: ## Front-end: test logic thuần bằng node --test
	cd frontend && node --test

test-firmware: ## Firmware: test lõi C++ trên máy tính (cần PlatformIO: pip install platformio)
	cd firmware && pio test -e native

e2e: ## Kiểm thử trình duyệt thật (cần: cd frontend && npm i && npx playwright install chromium)
	cd frontend && node e2e/smoke.mjs

build: ## Build backend thành một file chạy tĩnh: backend/bin/server
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/server ./cmd/server

up: ## Chạy Mosquitto + backend bằng Docker (xem deploy/docker-compose.yml)
	cd deploy && docker compose up -d --build

down: ## Dừng Docker
	cd deploy && docker compose down
