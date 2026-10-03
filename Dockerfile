# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/tariffCalculator .

FROM alpine:3.20
RUN adduser -D -u 1000 app
WORKDIR /app
COPY --from=build /out/tariffCalculator /app/tariffCalculator
# The server loads these via relative paths; drop these two lines once embed.FS lands.
COPY templates/ /app/templates/
COPY static/ /app/static/
ENV PORT=8080
EXPOSE 8080
USER app
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/ || exit 1
CMD ["/app/tariffCalculator"]
