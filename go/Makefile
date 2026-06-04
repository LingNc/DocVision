BINARY_NAME := docvision
BUILD_DIR := build
RELEASE_DIR := release
GO := go
GOFLAGS := -ldflags="-s -w"

.PHONY: build clean release test vet

build:
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/docvision/

release:
	@mkdir -p $(RELEASE_DIR)
	GOOS=linux   GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME)_linux_amd64 ./cmd/docvision/
	GOOS=linux   GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME)_linux_arm64 ./cmd/docvision/
	GOOS=darwin  GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME)_darwin_amd64 ./cmd/docvision/
	GOOS=darwin  GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME)_darwin_arm64 ./cmd/docvision/
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(RELEASE_DIR)/$(BINARY_NAME)_windows_amd64.exe ./cmd/docvision/
	@ls -la $(RELEASE_DIR)/

clean:
	rm -rf $(BUILD_DIR) $(RELEASE_DIR)

test:
	$(GO) test -v -race ./...

vet:
	$(GO) vet ./...
