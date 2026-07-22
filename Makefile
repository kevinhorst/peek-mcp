DIST    := dist
STAGE   := $(DIST)/bundle
LDFLAGS := -s -w
GOENV := GOOS=darwin CGO_ENABLED=0
VERSION = 1.0.6

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


build-local: clean-dist
	@mkdir -p $(DIST)
	go build -o dist/peek-mcp .


build-mcpb: build-darwin-universal sign-darwin build-mcpb-only

build-mcpb-only:
	@rm -rf $(STAGE) && mkdir -p $(STAGE)/server
	cp mcpb/manifest.json $(STAGE)/manifest.json
	cp $(DIST)/peek-mcp $(STAGE)/server/peek-mcp
	chmod +x $(STAGE)/server/peek-mcp
	cd $(STAGE) && zip -r ../peek-mcp.mcpb . -x '*.DS_Store'
	@echo "==> built $(DIST)/peek-mcp.mcpb"


clean-dist:
	rm -rf $(DIST)


git-release:
	sed -i '' 's/^VERSION = .*/VERSION = $(VERSION)/' Makefile
	sed -i '' 's/^var version = ".*"/var version = "$(VERSION)"/' cmd/version.go
	sed -i '' 's/^  "version": ".*",/  "version": "$(VERSION)",/' mcpb/manifest.json
	git commit -am "cmd: release v$(VERSION)"
	git tag v$(VERSION)


notarize-darwin:
	xcrun notarytool submit $(DIST)/peek-mcp.mcpb \
		--apple-id "$(APPLE_ID)" \
		--password "$(APPLE_APP_PASSWORD)" \
		--team-id "$(APPLE_TEAM_ID)" \
		--wait


serve-http: build-local
	./dist/peek-mcp start --log-level debug


serve-stdio: build-local
	./dist/peek-mcp start --transport stdio


sign-darwin:
	codesign --force --options runtime --sign - $(DIST)/peek-mcp


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
