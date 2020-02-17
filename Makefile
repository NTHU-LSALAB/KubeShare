TARGET=kubeshare-scheduler kubeshare-device-manager kubeshare-config-client
GO=go
GO_MODULE=GO111MODULE=on
BIN_DIR=bin/
ALPINE_COMPILE_FLAGS=CGO_ENABLED=0 GOOS=linux GOARCH=amd64
NVML_COMPILE_FLAGS=CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' GOOS=linux GOARCH=amd64
PACKAGE_PREFIX=github.com/NTHU-LSALAB/KubeShare/cmd/

.PHONY: all clean $(TARGET)

all: $(TARGET)

kubeshare-device-manager kubeshare-scheduler:
	$(GO_MODULE) $(ALPINE_COMPILE_FLAGS) $(GO) build -o $(BIN_DIR)$@ $(PACKAGE_PREFIX)$@

kubeshare-config-client:
	$(GO_MODULE) $(NVML_COMPILE_FLAGS) $(GO) build -o $(BIN_DIR)$@ $(PACKAGE_PREFIX)$@

clean:
	rm $(BIN_DIR)* 2>/dev/null; exit 0
