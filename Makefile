build:
	go build -o peek-mcp .

test:
	go test ./...

serve: build
	./peek-mcp
