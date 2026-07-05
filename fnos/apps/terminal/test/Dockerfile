FROM alpine:3.19

RUN apk add --no-cache bash

COPY test.sh /test.sh
RUN chmod +x /test.sh

CMD ["/test.sh"]
