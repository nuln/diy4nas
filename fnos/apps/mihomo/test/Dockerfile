FROM alpine:3.19

RUN apk add --no-cache curl

COPY server /app/server
COPY test.sh /test.sh

RUN chmod +x /app/server /test.sh

EXPOSE 9097

CMD ["/test.sh"]
