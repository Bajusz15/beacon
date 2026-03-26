FROM golang:1.26.1-alpine

RUN apk add --no-cache git bash curl expect docker-cli

WORKDIR /app

COPY . .

RUN go build -ldflags "-X beacon/internal/cloud.DefaultBeaconInfraAPIURL=https://beaconinfra.dev/api" -o beacon ./cmd/beacon

RUN cp beacon /usr/local/bin/beacon && chmod +x /usr/local/bin/beacon

COPY tests/e2e/test-cli.sh /app/test-cli.sh
COPY tests/e2e/test-docker.sh /app/test-docker.sh
COPY tests/e2e/git-http-server.sh /app/git-http-server.sh
RUN chmod +x /app/test-cli.sh /app/test-docker.sh /app/git-http-server.sh

ENV CI=true
ENV BEACON_E2E_TEST=1

EXPOSE 8080

CMD ["/app/test-cli.sh"]
