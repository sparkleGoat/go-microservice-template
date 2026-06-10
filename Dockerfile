# syntax=docker/dockerfile:1
#
# Multi-stage build producing a minimal, non-root, statically linked image.
# Final image is distroless (~2MB base) with no shell or package manager,
# shrinking the attack surface for production Kubernetes workloads.

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO disabled => fully static binary that runs on distroless/scratch.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/server ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/server /server

# Run as the built-in nonroot user (uid 65532); never root in production.
USER nonroot:nonroot

EXPOSE 8080 9090
ENTRYPOINT ["/server"]
