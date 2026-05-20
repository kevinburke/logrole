SHELL = /bin/bash -o pipefail

BUMP_VERSION := $(GOPATH)/bin/bump_version
DIFFER := $(GOPATH)/bin/differ
GO_BINDATA := $(GOPATH)/bin/go-bindata
JUSTRUN := $(GOPATH)/bin/justrun
WRITE_MAILMAP := $(GOPATH)/bin/write_mailmap
STATICCHECK := $(GOPATH)/bin/staticcheck

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
	static/css/style.css static/css/bootstrap.min.css

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
	go install github.com/kevinburke/go-bindata/v4/go-bindata@latest

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
