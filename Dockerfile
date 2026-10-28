# One build definition, four binaries (api, worker, outbox-publisher, consumer).
# All runtime binaries are statically linked and run as an unprivileged user.
ARG GO_VERSION=1.26.10
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG APP
RUN test -n "$APP" && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server "./cmd/${APP}"

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
USER app
COPY --from=builder /out/server /app/server
ENTRYPOINT ["/app/server"]
