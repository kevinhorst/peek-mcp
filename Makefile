DIST    := dist
STAGE   := $(DIST)/bundle
LDFLAGS := -s -w
GOENV := GOOS=darwin CGO_ENABLED=0

build-local: clean-dist
	@mkdir -p $(DIST)
	go build -o dist/peek-mcp .


build-darwin-universal:
	@mkdir -p $(DIST)
	$(GOENV)  GOARCH=arm64  go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-darwin-arm64 .
	$(GOENV)  GOARCH=amd64  go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-darwin-amd64 .
	lipo -create -output $(DIST)/peek-mcp $(DIST)/peek-mcp-darwin-arm64 $(DIST)/peek-mcp-darwin-amd64
	rm $(DIST)/peek-mcp-darwin-arm64 $(DIST)/peek-mcp-darwin-amd64


build-linux-amd64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-linux-amd64 .


build-linux-arm64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-linux-arm64 .


build-mcpb: build-darwin-universal
	@rm -rf $(STAGE) && mkdir -p $(STAGE)/server
	cp mcpb/manifest.json $(STAGE)/manifest.json
	cp $(DIST)/peek-mcp $(STAGE)/server/peek-mcp
	chmod +x $(STAGE)/server/peek-mcp
	cd $(STAGE) && zip -r ../peek-mcp.mcpb . -x '*.DS_Store'
	@echo "==> built $(DIST)/peek-mcp.mcpb"


clean-dist:
	rm -rf $(DIST)


git-release:
	git commit -am "cmd: release v$(VERSION)"
	git tag v$(VERSION)


serve-http: build-local
	./dist/peek-mcp start --log-level debug


serve-stdio: build-local
	./dist/peek-mcp start --transport stdio


test:
	go test ./...


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
