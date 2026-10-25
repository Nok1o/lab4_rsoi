FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/library-service ./library_service/cmd/library-service

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=build /out/library-service /usr/local/bin/library-service

EXPOSE 8060
ENTRYPOINT ["library-service"]
