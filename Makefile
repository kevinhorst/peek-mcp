DIST    := dist
STAGE   := $(DIST)/bundle
LDFLAGS := -s -w
GOENV := GOOS=darwin CGO_ENABLED=0

build:
	go build -o peek-mcp .

build-darwin-universal:
	@mkdir -p $(DIST)
	$(GOENV)  GOARCH=arm64  go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-darwin-arm64 .
	$(GOENV)  GOARCH=amd64  go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-darwin-amd64 .
	lipo -create -output $(DIST)/peek-mcp $(DIST)/peek-mcp-darwin-arm64 $(DIST)/peek-mcp-darwin-amd64
	rm $(DIST)/peek-mcp-darwin-arm64 $(DIST)/peek-mcp-darwin-amd64


clean-dist:
	rm -rf $(DIST)

mcpb: build-darwin-universal
	@rm -rf $(STAGE) && mkdir -p $(STAGE)/server
	cp mcpb/manifest.json $(STAGE)/manifest.json
	cp $(DIST)/peek-mcp $(STAGE)/server/peek-mcp
	chmod +x $(STAGE)/server/peek-mcp
	cd $(STAGE) && zip -r ../peek-mcp.mcpb . -x '*.DS_Store'
	@echo "==> built $(DIST)/peek-mcp.mcpb"

test:
	go test ./...

serve-http: build
	./peek-mcp start

serve-stdio: build
	./peek-mcp start --transport stdio


update-go-deps:
	@echo ">> updating Go dependencies"
	@for m in $$(go list -mod=readonly -m -f '{{ if and (not .Indirect) (not .Main)}}{{.Path}}{{end}}' all); do \
		go get $$m; \
	done
	go mod tidy
ifneq (,$(wildcard vendor))
	go mod vendor
endif
	@echo "✓ Dependencies updated!"
