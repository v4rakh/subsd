APP       := subsd
CMD       := ./cmd/subsd

PNPM      ?= pnpm
GRYPE     ?= grype

BINDIR             := bin
BINARY             := $(BINDIR)/$(APP)
FRONTEND_DIR       := $(shell pwd)/frontend
FRONTEND_NODE_DIR  := $(shell pwd)/frontend/node_modules
FRONTEND_CI_DIR    := $(shell pwd)/frontend/ci/*.xml
DIST               := ./web/dist

LDFLAGS   := -ldflags "-s -w"

.PHONY: build build-frontend build-server build-all build-server-all build-server-freebsd-amd64 build-server-freebsd-arm64 build-server-darwin-amd64 build-server-darwin-arm64 build-server-linux-amd64 build-server-linux-arm64 clean dependencies dependencies-frontend dependencies-server checkstyle checkstyle-server checkstyle-frontend checkstyle-fix checkstyle-server-fix checkstyle-frontend-fix scan test test-server test-frontend

build: build-frontend build-server

build-server:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

build-frontend:
	cd $(FRONTEND_DIR) && $(PNPM) run build

build-all: build-frontend build-server-all

build-server-all:  build-server-freebsd-amd64 build-server-freebsd-arm64 build-server-darwin-amd64 build-server-darwin-arm64 build-server-linux-amd64 build-server-linux-arm64

build-server-freebsd-amd64:
	CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(BINARY)-freebsd-amd64 $(CMD)
build-server-freebsd-arm64:
	CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(BINARY)-freebsd-arm64 $(CMD)
build-server-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(BINARY)-darwin-amd64 $(CMD)
build-server-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(BINARY)-darwin-arm64 $(CMD)
build-server-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(BINARY)-linux-amd64 $(CMD)
build-server-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(BINARY)-linux-arm64 $(CMD)

dependencies: dependencies-frontend dependencies-server

dependencies-server:
	go mod download

dependencies-frontend:
	cd $(FRONTEND_DIR) && $(PNPM) install --frozen-lockfile

test: test-server test-frontend

test-server:
	go test -cover -race -shuffle on -v ./...

test-frontend:
	cd $(FRONTEND_DIR) && $(PNPM) run test:coverage

checkstyle: checkstyle-frontend checkstyle-server

checkstyle-server:
	golangci-lint cache clean
	golangci-lint run

checkstyle-frontend:
	cd $(FRONTEND_DIR) && $(PNPM) run checkstyle

checkstyle-fix: checkstyle-frontend-fix checkstyle-server-fix

checkstyle-server-fix:
	golangci-lint cache clean
	golangci-lint run --fix

checkstyle-frontend-fix:
	cd $(FRONTEND_DIR) && $(PNPM) run format && $(PNPM) run lint:fix && $(PNPM) run i18n-sync && $(PNPM) run lint:style:fix

scan:
	@NO_COLOR=1 $(GRYPE) -v -o table --file bin/grype.txt --fail-on critical bin/ || true
	@cat ./bin/grype.txt

clean:
	rm -rf $(BINDIR)
	rm -rf $(DIST)
	rm -rf $(FRONTEND_NODE_DIR)
	rm -rf $(FRONTEND_CI_DIR)
	rm -f $(FRONTEND_DIR)/.eslintcache
	rm -f $(FRONTEND_DIR)/.stylelintcache
