PATH := $(CURDIR)/bin:$(PATH)
BUILD_DIR := bin
COMPILE_FLAGS := CGO_ENABLED=1 CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' GOOS=linux GOARCH=amd64
REGISTRY := ghcr.io
REPOSITORY := nthu-lsalab/kubeshare
IMAGE_NAME := $(REGISTRY)/$(REPOSITORY)
IMAGE_TAG := $(shell git rev-parse HEAD)

.PHONY: clean
clean:
	rm -rf bin/*

.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	$(COMPILE_FLAGS) go build -o $(BUILD_DIR)/kubeshare ./cmd/.

.PHONY: build-push-image
build-push-image:
	docker build -t $(IMAGE_NAME):latest -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker push $(IMAGE_NAME):latest
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
