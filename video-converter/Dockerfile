FROM golang:1.23-alpine

RUN apk add --no-cache git bash ffmpeg

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .


CMD [ "bash" ]
