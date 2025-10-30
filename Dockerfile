FROM golang:1.24.4-alpine AS builder
RUN apk add --no-cache git

WORKDIR /app

COPY . .

RUN go mod tidy

RUN go build -o pipedrive_api_service ./cmd/main.go

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/pipedrive_api_service .

EXPOSE 9010

CMD ["./pipedrive_api_service"]
