FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /loadbalancer ./cmd/loadbalancer
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /example-backend ./cmd/example-backend

# --- Final image ---
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=builder /loadbalancer /usr/local/bin/loadbalancer
COPY --from=builder /example-backend /usr/local/bin/example-backend

EXPOSE 8080 9090

ENTRYPOINT ["loadbalancer"]
CMD ["-config", "/etc/loadbalancer/config.yaml"]
