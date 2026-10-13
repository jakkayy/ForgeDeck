# syntax=docker/dockerfile:1
#
# The ForgeDeck Control Plane: the API and the dashboard in one binary.
# Build from the repository root:
#   docker build -f deploy/Dockerfile --build-arg VERSION=$(git describe --tags --always) -t forgedeck-api .

# --- the dashboard (Bun is needed only here, never at run time) ---
FROM oven/bun:1.3.9 AS web
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/bun.lock ./
RUN bun install --frozen-lockfile
COPY apps/web ./
RUN bun run build

# --- the server, with the dashboard embedded ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/apps/web/dist/ internal/webui/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/forgedeck-api ./cmd/api

# --- the image that runs: a static binary, no shell, no package manager, not root ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/forgedeck-api /forgedeck-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/forgedeck-api"]
CMD ["serve"]
