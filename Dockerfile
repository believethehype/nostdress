FROM golang:1.27.3-alpine3.24 AS builder

WORKDIR /opt/build

COPY ./*.go ./*.html ./go.mod ./go.sum ./
COPY static ./static

RUN apk update && \
    apk upgrade --available && \
    apk add gcc musl-dev linux-headers
RUN go get
RUN go build

FROM alpine:3.24.2

ENV PORT=17422
ENV DOMAIN=satdress.com
ENV SECRET=askdbasjdhvakjvsdjasd
ENV SITE_OWNER_URL=https://t.me/fiatjaf
ENV SITE_OWNER_NAME=@fiatjaf
ENV SITE_NAME=Nostdress

COPY --from=builder /opt/build/nostdress /usr/local/bin/

EXPOSE 17422

CMD ["nostdress"]
