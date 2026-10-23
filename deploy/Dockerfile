# syntax=docker/dockerfile:1
# The ForgeDeck Control Plane: the API and the dashboard in one binary.
FROM --platform=$BUILDPLATFORM oven/bun:1.3.9 AS web
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/bun.lock ./
RUN bun install --frozen-lockfile
COPY apps/web ./
RUN bun run build

# Built on the builder's own architecture and cross-compiled, so arm64 images need no emulation.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/apps/web/dist/ internal/webui/dist/
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/forgedeck-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/forgedeck-api /forgedeck-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/forgedeck-api"]
CMD ["serve"]
