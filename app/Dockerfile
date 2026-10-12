# syntax=docker/dockerfile:1

# ---- build: compile a static binary and run the tests -------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod ./
RUN go mod download
COPY . .
RUN go vet ./... && go test ./...
ARG VERSION=dev
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/status-check .

# ---- runtime: distroless, non-root, no shell -----------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/status-check /status-check
# Default config so the image runs anywhere; mount your own over it.
COPY checks.example.json /etc/status-check/checks.json
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD ["/status-check", "-healthcheck"]
ENTRYPOINT ["/status-check"]
