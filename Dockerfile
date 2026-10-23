# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# The tests run in CI before an image is used (they ran here too, so three
# times a deploy: CI, CI's image build and the server's), so not again here.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/tariffCalculator .

FROM alpine:3.20
RUN adduser -D -u 1000 app
WORKDIR /app
COPY --from=build /out/tariffCalculator /app/tariffCalculator
ENV PORT=8080
EXPOSE 8080
USER app
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/tariffCalculator"]
