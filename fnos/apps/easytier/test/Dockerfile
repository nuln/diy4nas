FROM alpine:3.19

RUN apk add --no-cache curl bash

COPY server /app/server
COPY test_basic.sh /test.sh

RUN chmod +x /app/server /test.sh

EXPOSE 11215

CMD ["/test.sh"]