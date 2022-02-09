# Build the forwarder in a stock Go builder container
FROM golang:alpine as builder

ADD . /grafana-threema-forwarder
RUN cd /grafana-threema-forwarder && go build .

# Pull the forwarder into a second stage deploy alpine container
FROM alpine:latest

COPY --from=builder /grafana-threema-forwarder/grafana-threema-forwarder /usr/local/bin/

EXPOSE 8000
ENTRYPOINT ["grafana-threema-forwarder"]
