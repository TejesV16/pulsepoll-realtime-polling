FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pulsepoll ./cmd/server

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
USER app
COPY --from=build /pulsepoll /pulsepoll
EXPOSE 8080
CMD ["/pulsepoll"]
