# would be great to make the bash location portable but not sure how
SHELL = /bin/bash

BUMP_VERSION := $(GOPATH)/bin/bump_version
GODOCDOC := $(GOPATH)/bin/godocdoc
GO_BINDATA := $(GOPATH)/bin/go-bindata
DEP := $(GOPATH)/bin/dep
JUSTRUN := $(GOPATH)/bin/justrun
BENCHSTAT := $(GOPATH)/bin/benchstat
WRITE_MAILMAP := $(GOPATH)/bin/write_mailmap
MEGACHECK := $(GOPATH)/bin/megacheck

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
	templates/calls/list.html templates/calls/instance.html \
	templates/calls/recordings.html \
	templates/conferences/list.html templates/conferences/instance.html \
	templates/alerts/list.html templates/alerts/instance.html \
	templates/phone-numbers/list.html \
	templates/snippets/phonenumber.html \
	templates/errors.html templates/login.html \
	static/css/style.css static/css/bootstrap.min.css

test: vet
	@# this target should always be listed first so "make" runs the tests.
	go list ./... | grep -v vendor | xargs go test -short

race-test: vet
	go list ./... | grep -v vendor | xargs go test -race

serve:
	go run commands/logrole_server/main.go

$(MEGACHECK):
	go get honnef.co/go/tools/cmd/megacheck

vet: $(MEGACHECK)
	@# We can't vet the vendor directory, it fails.
	go list ./... | grep -v vendor | xargs go vet
	go list ./... | grep -v vendor | xargs $(MEGACHECK) --ignore='github.com/kevinburke/logrole/*/*.go:S1002'

deploy:
	git push heroku master

compile-css: static/css/bootstrap.min.css static/css/style.css
	cat static/css/bootstrap.min.css static/css/style.css > static/css/all.css

$(GO_BINDATA):
	go get -u github.com/jteeuwen/go-bindata/...

assets: $(ASSET_TARGETS) compile-css | $(GO_BINDATA)
	go-bindata -o=assets/bindata.go --nometadata --pkg=assets templates/... static/...

$(JUSTRUN):
	go get -u github.com/jmhodges/justrun

watch: | $(JUSTRUN)
	$(JUSTRUN) -v --delay=100ms -c 'make assets serve' $(WATCH_TARGETS)

$(DEP):
	go get -u github.com/golang/dep/cmd/dep

deps: | $(DEP)
	$(DEP) ensure
	$(DEP) prune

$(BUMP_VERSION):
	go get github.com/Shyp/bump_version

$(GODOCDOC):
	go get github.com/kevinburke/godocdoc

release: race-test | $(BUMP_VERSION) $(DIFFER)
	$(DIFFER) $(MAKE) authors
	bump_version minor http.go

docs: | $(GODOCDOC)
	$(GODOCDOC)

$(BENCHSTAT):
	go get golang.org/x/perf/cmd/benchstat

bench: $(BENCHSTAT)
	tmp=$$(mktemp); go list ./... | grep -v vendor | xargs go test -benchtime=2s -bench=. -run='^$$' > "$$tmp" 2>&1 && $(BENCHSTAT) "$$tmp"

loc:
	cloc --exclude-dir=.git,tmp,vendor --not-match-f='bootstrap.min.css|all.css|bindata.go' .

# For Travis. Run the tests with unvendored dependencies, just check the latest
# version of everything out to the GOPATH.
unvendored:
	rm -rf vendor/*/
	go get -t -u ./...
	$(MAKE) race-test
	$(DEP) ensure

$(WRITE_MAILMAP):
	go get github.com/kevinburke/write_mailmap

AUTHORS.txt: | $(WRITE_MAILMAP)
	$(WRITE_MAILMAP) > AUTHORS.txt

authors: AUTHORS.txt
	write_mailmap > AUTHORS.txt
