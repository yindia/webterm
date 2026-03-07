FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/webterm ./cmd/webterm

FROM alpine:3.20
RUN apk add --no-cache bash ca-certificates
WORKDIR /app
COPY --from=builder /out/webterm /usr/local/bin/webterm
EXPOSE 8080
ENTRYPOINT ["webterm"]
CMD ["serve", "--config", "/app/webterm.yaml"]
