# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source, build script and run it
# NOTE: please ensure that file list to copy is synced with Dockerfile.multi-arch
COPY main.go sidecar.go handler.go class_data.go errors.go watcher.go updater.go ./
COPY scripts/build-manager.sh /build-manager.sh
RUN /build-manager.sh

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
