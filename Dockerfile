FROM golang:alpine

STOPSIGNAL SIGKILL
ENTRYPOINT /usr/app/bot
EXPOSE 8443
ENV GOPATH="/usr/app/"

COPY . /usr/app/src/google-play-review-bot/
RUN apk add --no-cache git
RUN cd /usr/app/src/google-play-review-bot && go get && go build -o /usr/app/bot && rm -r /usr/app/src && apk del git
