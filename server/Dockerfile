FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    harfbuzz \
    font-noto-cjk

ENV CHROME_PATH=/usr/bin/chromium-browser
ENV CHROMEDP_HEADLESS=true

COPY --from=builder /server /server

ENTRYPOINT ["/server"]
