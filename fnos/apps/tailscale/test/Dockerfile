FROM alpine:3.19

RUN apk add --no-cache curl bash

COPY server /app/server
COPY mock-tailscaled /app/tailscaled
COPY mock-tailscale /app/tailscale
COPY test.sh /test.sh

RUN mkdir -p /data && chmod +x /app/server /app/tailscaled /app/tailscale /test.sh

EXPOSE 8088

CMD ["/test.sh"]
