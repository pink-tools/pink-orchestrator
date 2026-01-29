VERSION := dev-$(shell date +%Y-%m-%d_%H:%M:%S)
INSTALL_DIR := ~/pink-tools/pink-orchestrator

build:
	go build -ldflags="-X main.version=$(VERSION)" -o pink-orchestrator .

install: build
	cp pink-orchestrator $(INSTALL_DIR)/pink-orchestrator
