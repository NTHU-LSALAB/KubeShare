PATH := $(CURDIR)/bin:$(PATH)
BUILD_DIR := bin
COMPILE_FLAGS := CGO_ENABLED=1 CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' GOOS=linux GOARCH=amd64

.PHONY: clean
clean:
	rm -rf bin/*

.PHONY: dc.build
dc.build:
	docker-compose run --rm build

.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	$(COMPILE_FLAGS) go build -o $(BUILD_DIR)/cmd ./cmd/.
