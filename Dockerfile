# syntax=docker/dockerfile:1

# 1. Frontend
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Backend (embeds web/dist/spa)
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist/spa ./web/dist/spa
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tidemark ./cmd/tidemark \
 && mkdir -p /out/data

# 3. Runtime: no shell, non-root (uid 65532)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tidemark /tidemark
COPY --from=build --chown=65532:65532 /out/data /data
ENV TIDEMARK_DATA_DIR=/data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/tidemark", "healthcheck"]
ENTRYPOINT ["/tidemark"]
