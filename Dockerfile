FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o athena-server .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/athena-server .

RUN mkdir /app/data

EXPOSE 8080

CMD ["./athena-server"]
