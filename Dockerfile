FROM golang:1.25-alpine AS builder
ARG VERSION=1.1.2
ARG BUILD_DATE=02.03.2026

WORKDIR /app
COPY go.mod ./ 

RUN if [ -f go.sum ]; then cp go.sum .; fi
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags="-w -s -X main.version=$VERSION -X main.buildDate=$BUILD_DATE" \
  -o server ./cmd/server/main.go

FROM alpine:3.18
ARG VERSION=0.1
ARG BUILD_DATE=07.11.2025
LABEL version=$VERSION \
  build-date=$BUILD_DATE \
  maintainer="me@xomrkob.ru"
RUN addgroup -g 1000 appgroup && \
  adduser -D -u 1000 -G appgroup appuser
WORKDIR /app 
COPY --from=builder --chown=appuser:appgroup /app/server .
COPY --from=builder --chown=appuser:appgroup /app/config ./config
EXPOSE 8080

USER appuser
CMD ["/app/server"]
