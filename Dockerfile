# Multi-stage build for the single pp-vpa binary. The same image runs in two
# modes via the --mode flag (manager or node-agent), so the Helm chart can
# point both the Deployment and the DaemonSet at this image.
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-s -w" -o pp-vpa cmd/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/pp-vpa /pp-vpa
USER 65532:65532

ENTRYPOINT ["/pp-vpa"]
