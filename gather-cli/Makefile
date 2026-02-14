.PHONY: build install clean codegen release

build:
	go build -o gather .

install: build
	mkdir -p $(HOME)/.local/bin
	cp gather $(HOME)/.local/bin/

clean:
	rm -f gather

# Cross-compile for release
release:
	GOOS=linux GOARCH=amd64 go build -o gather-linux-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o gather-darwin-arm64 .

# Regenerate client types from the live OpenAPI spec.
# Requires: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
codegen:
	curl -s https://gather.is/openapi.json -o openapi.json
	python3 scripts/downgrade_spec.py openapi.json openapi30.json
	oapi-codegen -config codegen.yaml openapi30.json
	rm -f openapi.json openapi30.json
	@echo "types_gen.go updated"
