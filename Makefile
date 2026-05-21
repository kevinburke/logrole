SHELL = /bin/bash -o pipefail

# `go install` writes to GOBIN when it's set, otherwise to GOPATH/bin.
# Mirror that here so the file targets point at the binary's actual
# install location - relying on `$(GOPATH)` alone breaks in CI
# environments that set GOBIN but not GOPATH (e.g. Buildkite).
GO_BIN_DIR := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

BUMP_VERSION := $(GO_BIN_DIR)/bump_version
DIFFER := $(GO_BIN_DIR)/differ
GO_BINDATA := $(GO_BIN_DIR)/go-bindata
JUSTRUN := $(GO_BIN_DIR)/justrun
WRITE_MAILMAP := $(GO_BIN_DIR)/write_mailmap
STATICCHECK := $(GO_BIN_DIR)/staticcheck
NPM := npm

WATCH_TARGETS = static/css/style.css \
	templates/base.html \
	templates/phone-numbers/list.html templates/phone-numbers/instance.html \
	templates/conferences/instance.html templates/conferences/list.html \
	templates/alerts/list.html templates/alerts/instance.html \
	templates/errors.html templates/login.html \
	templates/snippets/phonenumber.html \
	services/error_reporter.go services/services.go \
	server/calls.go server/alerts.go server/phonenumbers.go \
	server/serve.go server/render.go views/client.go views/numbers.go \
	Makefile config.yml

ASSET_TARGETS = templates/base.html templates/index.html \
	templates/messages/list.html templates/messages/instance.html \
	templates/messages/new.html \
	templates/calls/list.html templates/calls/instance.html \
	templates/calls/recordings.html \
	templates/conferences/list.html templates/conferences/instance.html \
	templates/alerts/list.html templates/alerts/instance.html \
	templates/phone-numbers/list.html \
	templates/snippets/phonenumber.html \
	templates/errors.html templates/login.html \
	static/css/style.css static/css/bootstrap.min.css \
	static/js/twilio-voice-sdk.js

.PHONY: test race-test serve lint assets watch release docs bench loc authors ci

test: lint
	@# this target should always be listed first so "make" runs the tests.
	go test -trimpath -short ./...

race-test: lint
	go test -trimpath -race ./...

serve:
	go run -trimpath ./commands/logrole_server

$(STATICCHECK):
	go install honnef.co/go/tools/cmd/staticcheck@latest

lint: | $(STATICCHECK)
	go vet ./...
	$(STATICCHECK) --checks='["all", "-ST1005", "-S1002"]' ./...

compile-css: static/css/bootstrap.min.css static/css/style.css
	cat static/css/bootstrap.min.css static/css/style.css > static/css/all.css

$(GO_BINDATA):
	go install github.com/kevinburke/go-bindata/v4/go-bindata@v4.0.2

node_modules: package-lock.json
	$(NPM) ci
	@touch node_modules

# Bundle browser JS. Currently this is just the Twilio Voice JS SDK
# repackaged to expose Twilio.Device on window. Sourcemap is intentionally
# off because the bundle is embedded into the Go binary via go-bindata.
static/js/twilio-voice-sdk.js: js/twilio-voice-sdk.js package-lock.json | node_modules
	$(NPM) run build

# `make assets` regenerates the embedded bindata. The output is not
# byte-stable across Go toolchain versions: compress/gzip in 1.27
# emits slightly different bytes than 1.26 for the same input, and
# bindata.go is gzip-compressed. CI runs Go 1.26, so the committed
# bindata.go must be produced with a go-bindata built under Go 1.26
# or the Assets CI step will report drift. If you have multiple Go
# toolchains installed:
#
#   GOROOT=~/go1.26 PATH=~/go1.26/bin:$PATH \
#       go install github.com/kevinburke/go-bindata/v4/go-bindata@v4.0.2
#   make assets
assets: $(ASSET_TARGETS) compile-css | $(GO_BINDATA)
	$(GO_BINDATA) -o=assets/bindata.go --nometadata --pkg=assets templates/... static/...

$(JUSTRUN):
	go install github.com/jmhodges/justrun@latest

watch: | $(JUSTRUN)
	$(JUSTRUN) -v --delay=100ms -c 'make assets serve' $(WATCH_TARGETS)

$(BUMP_VERSION):
	go install github.com/kevinburke/bump_version@latest

$(DIFFER):
	go install github.com/kevinburke/differ@latest

version ?= minor

release: race-test | $(BUMP_VERSION) $(DIFFER)
	$(DIFFER) $(MAKE) authors
	$(BUMP_VERSION) --tag-prefix=v $(version) server/serve.go

bench:
	tmp=$$(mktemp); go test -trimpath -benchtime=2s -bench=. -run='^$$' ./... > "$$tmp" 2>&1 && go run golang.org/x/perf/cmd/benchstat@latest "$$tmp"

loc:
	cloc --exclude-dir=.git,tmp,vendor,worktrees --not-match-f='bootstrap.min.css|all.css|bindata.go' .

ci: race-test bench

$(WRITE_MAILMAP):
	go install github.com/kevinburke/write_mailmap@latest

AUTHORS.txt: | $(WRITE_MAILMAP)
	$(WRITE_MAILMAP) > AUTHORS.txt

authors: AUTHORS.txt
	$(WRITE_MAILMAP) > AUTHORS.txt
