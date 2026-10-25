FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/gateway-service ./gateway_service/cmd/gateway-service

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=build /out/gateway-service /usr/local/bin/gateway-service

EXPOSE 8080
ENTRYPOINT ["gateway-service"]
