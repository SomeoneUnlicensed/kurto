BINARY = kurto
VERSION = $(shell git describe --tags --always 2>/dev/null || echo "dev")

.PHONY: all build vet clean dist

all: vet build

build:
	go build -ldflags="-X main.version=$(VERSION)" -o $(BINARY) .

vet:
	go vet ./...

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf dist/

dist: clean
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-linux-arm64 .
	GOOS=linux   GOARCH=arm   go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-linux-arm .
	GOOS=linux   GOARCH=386   go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-linux-386 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-windows-amd64.exe .
	GOOS=windows GOARCH=386   go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-windows-386.exe .
	GOOS=darwin  GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="-X main.version=$(VERSION)" -o dist/$(BINARY)-darwin-arm64 .
	cd dist; \
		for f in $(BINARY)-linux-*; do gzip -c "$$f" > "$$f.gz"; done; \
		for f in $(BINARY)-windows-*.exe; do zip "$${f%.exe}.zip" "$$f"; done; \
		for f in $(BINARY)-darwin-*; do gzip -c "$$f" > "$$f.gz"; done
	ls -la dist/
