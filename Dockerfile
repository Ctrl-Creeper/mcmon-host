FROM alpine:3.21

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN apk add --no-cache ca-certificates tzdata

COPY dist/mcmon-host-${TARGETOS}-${TARGETARCH} /usr/local/bin/mcmon-host
RUN chmod 0755 /usr/local/bin/mcmon-host

RUN mkdir -p /data
WORKDIR /data

EXPOSE 9090
VOLUME ["/data"]

CMD ["/usr/local/bin/mcmon-host", "-config", "/data/config.json"]
