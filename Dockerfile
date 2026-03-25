# --- BUILD STAGE ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o app ./cmd/main.go

# --- FINAL STAGE ---
FROM alpine:3.19

WORKDIR /root/

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/app .

CMD ["./app"]
