ARG alpine_tag=3.22
ARG go_tag=1.24.4-alpine3.22

FROM golang:${go_tag} AS builder
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOCACHE=/root/.cache/go-build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target="/root/.cache/go-build" go build -o /go/bin/alertmanager_max_webhook

FROM alpine:${alpine_tag}
WORKDIR /app
COPY --from=builder /go/bin/alertmanager_max_webhook /app/alertmanager_max_webhook
ENTRYPOINT ["/app/alertmanager_max_webhook"]
CMD ["-listen-address=:9096"]
