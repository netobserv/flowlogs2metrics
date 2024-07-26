# We do not use --platform feature to auto fill this ARG because of incompatibility between podman and docker
ARG TARGETPLATFORM=linux/amd64
ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.22 as builder

ARG TARGETPLATFORM
ARG TARGETARCH=amd64
WORKDIR /app

# Copy source code
COPY go.mod .
COPY go.sum .
COPY Makefile .
COPY .mk/ .mk/
COPY .bingo/ .bingo/
COPY vendor/ vendor/
COPY .git/ .git/
COPY cmd/ cmd/
COPY pkg/ pkg/

RUN git status --porcelain
RUN GOARCH=$TARGETARCH make build_k8s_cache

# final stage
FROM  --platform=$TARGETPLATFORM registry.access.redhat.com/ubi9/ubi-minimal:9.4

COPY --from=builder /app/k8s-cache /app/

ENTRYPOINT ["/app/k8s-cache"]
