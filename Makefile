.PHONY: all check lint test race vuln build install release format tidy vet staticcheck map version bump commit push ci checkpoint clean audit symbols template suggest-split visual review-loop snapshot snap

all: check

visual:
	@./tools/visual_audit

check:
	@./tools/check

ci:
	@./tools/check --full

lint:
	@./tools/lint

audit:
	@go-audit .

symbols:
	@./tools/outline_symbols $(ARGS)

suggest-split:
	@./tools/suggest_split $(ARGS)

template:
	@./tools/generate_config_template

test:
	@go test -race -timeout 180s ./...

race:
	@go test -race -timeout 180s ./...

vuln:
	@$$HOME/.go/bin/govulncheck ./...

build:
	@./tools/build

format:
	@./tools/format.sh

tidy:
	@go mod tidy

vet:
	@go vet ./...

staticcheck:
	@staticcheck -checks 'inherit,-SA2001' ./...

map:
	@./tools/map.sh

version:
	@./tools/version.sh

bump:
	@./tools/bump

commit:
	@./tools/commit $(ARGS)

push: bump

install:
	@./tools/install

release:
	@./tools/release $(ARGS)

snapshot:
	@./tools/snapshot $(ARGS)

snap: snapshot

checkpoint: snapshot

review-loop:
	@./tools/review_loop $(ARGS)

clean:
	@rm -f pod abs
	@echo "Cleaned."
