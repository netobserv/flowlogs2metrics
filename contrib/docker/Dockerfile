ARG TARGETARCH
FROM docker.io/library/golang:1.23 as builder

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
RUN GOARCH=$TARGETARCH make build_code

# final stage
FROM --platform=linux/$TARGETARCH registry.access.redhat.com/ubi9/ubi-minimal:9.5

COPY --from=builder /app/flowlogs-pipeline /app/
COPY --from=builder /app/confgenerator /app/

# expose ports
EXPOSE 2055

ENTRYPOINT ["/app/flowlogs-pipeline"]
