.DEFAULT_GOAL := help

GO ?= go
BIN_DIR := bin
CONTROLPLANE_BIN := $(BIN_DIR)/frantransport-controlplane
DAEMON_BIN := $(BIN_DIR)/frantransportd

.PHONY: help build fmt vet test test-race check tailcat-integration run-controlplane run-node clean

help: ## Show available targets.
	@printf '%s\n' \
	  'FranTransport targets:' \
	  '  make build                Build both commands into ./bin' \
	  '  make fmt                  Format all Go packages' \
	  '  make vet                 Run Go static analysis' \
	  '  make test                Run the hermetic test suite' \
	  '  make test-race           Run tests with the race detector' \
	  '  make check               Run fmt, vet, tests, and race tests' \
	  '  make tailcat-integration Run the real public-DERP contract test' \
	  '  make run-controlplane    Run the local control plane' \
	  '  make run-node            Run frantransportd' \
	  '  make clean               Remove built command binaries'

build: ## Build both commands.
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(CONTROLPLANE_BIN) ./cmd/frantransport-controlplane
	$(GO) build -o $(DAEMON_BIN) ./cmd/frantransportd

fmt: ## Format all Go packages.
	$(GO) fmt ./...

vet: ## Run Go static analysis.
	$(GO) vet ./...

test: ## Run tests that do not require public network access.
	$(GO) test ./...

test-race: ## Run all hermetic tests with the race detector.
	$(GO) test -race ./...

check: fmt vet test test-race ## Run the complete local quality gate.

tailcat-integration: ## Exercise Tailcat against the public DERP infrastructure.
	FRANTRANSPORT_TAILCAT_INTEGRATION=1 $(GO) test ./pkg/transport/tailcat -run TestContract -count=1 -timeout=90s -v

run-controlplane: ## Run the control plane with its default local settings.
	$(GO) run ./cmd/frantransport-controlplane

run-node: ## Run frantransportd; set FRANTRANSPORT_ENROLLMENT_TOKEN for first enrollment.
	$(GO) run ./cmd/frantransportd

clean: ## Remove binaries produced by make build.
	rm -f ./bin/frantransport-controlplane ./bin/frantransportd
	@rmdir ./bin 2>/dev/null || true
