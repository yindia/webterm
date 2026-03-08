APP_NAME ?= webterm
BINARY ?= $(APP_NAME)
IMAGE ?= $(APP_NAME):latest

.PHONY: help deps frontend-build frontend-sync build test doctor run image clean

help:
	@printf "Targets:\n"
	@printf "  make deps           Install frontend deps\n"
	@printf "  make frontend-build Build Next.js static export\n"
	@printf "  make frontend-sync  Copy frontend export to web/dist\n"
	@printf "  make build          Build Go binary with embedded assets\n"
	@printf "  make test           Run Go tests\n"
	@printf "  make doctor         Run webterm doctor\n"
	@printf "  make run            Run webterm serve\n"
	@printf "  make image          Build Docker image (pre-runs frontend export + sync)\n"
	@printf "  make clean          Clean generated artifacts\n"

deps:
	npm --prefix frontend install

frontend-build: deps
	rm -rf frontend/.next frontend/out
	npm --prefix frontend run build

build: frontend-build
	go build -tags embedout -o $(BINARY) .

test:
	go test ./...

doctor: build
	./$(BINARY) doctor --config ./webterm.yaml

run: build
	./$(BINARY) serve --config ./webterm.yaml

image: frontend-sync
	docker build -t $(IMAGE) .

clean:
	rm -rf frontend/.next frontend/out web/dist
	rm -f $(BINARY)
