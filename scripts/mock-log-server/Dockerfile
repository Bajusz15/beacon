FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod main.go ./
RUN go build -o mock-log-server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /app/mock-log-server /mock-log-server
EXPOSE 8080
ENTRYPOINT ["/mock-log-server"]
