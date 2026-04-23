GO ?= go

.PHONY: generate unit integration test

generate:
	@$(GO) generate ./...

unit: generate
	$(GO) test ./...

integration:
	$(GO) test -tags=integration ./testing/...

test: unit integration
