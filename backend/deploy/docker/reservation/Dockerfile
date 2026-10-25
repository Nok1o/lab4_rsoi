FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/reservation-service ./reservation_service/cmd/reservation-service

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=build /out/reservation-service /usr/local/bin/reservation-service

EXPOSE 8070
ENTRYPOINT ["reservation-service"]
