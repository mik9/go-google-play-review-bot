FROM golang:alpine AS builder

RUN apk add --no-cache git
WORKDIR /src/google-play-review-bot/

COPY go.mod go.sum /src/google-play-review-bot/
RUN go mod download
COPY . /src/google-play-review-bot/
RUN go build -o /usr/app/bot

FROM alpine:latest
RUN apk --no-cache add ca-certificates
STOPSIGNAL SIGKILL
ENTRYPOINT /app
EXPOSE 8443
ENV GOPATH="/usr/app/"

COPY --from=builder /usr/app/bot /app