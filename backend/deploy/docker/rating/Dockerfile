FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/rating-service ./rating_service/cmd/rating-service

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=build /out/rating-service /usr/local/bin/rating-service

EXPOSE 8050
ENTRYPOINT ["rating-service"]
