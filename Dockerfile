FROM golang:latest as backend-builder
ENV CGO_ENABLED=0
ENV GOPROXY=https://proxy.golang.org,direct
WORKDIR /app
COPY . .
RUN go build \
  -a \
  -trimpath \
  -ldflags "-s -w -extldflags='-static'" \
  -o "metrics_server" \
  ./cmd/main.go

EXPOSE 8080/tcp

CMD ./metrics_server
