FROM golang:alpine AS builder

RUN apk add --no-cache git

COPY . /go/src/google-play-review-bot/
RUN cd /go/src/google-play-review-bot && go get -d ./... && go build -o /usr/app/bot

FROM alpine:latest
RUN apk --no-cache add ca-certificates
STOPSIGNAL SIGKILL
ENTRYPOINT /app
EXPOSE 8443
ENV GOPATH="/usr/app/"

COPY --from=builder /usr/app/bot /app