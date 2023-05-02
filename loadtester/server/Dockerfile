FROM golang:1.20.3 AS build-env

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /go/src/github.com/mercari/tortoise/loadtester/server

COPY . .
RUN go build -o ./loadtester-server .

FROM alpine:latest

COPY --from=build-env /go/src/github.com/mercari/tortoise/loadtester/server/loadtester-server /bin/loadtester-server

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1001 app && adduser -D -G app -u 1001 app

RUN chmod +x /bin/loadtester-server

USER 1001

CMD ["/bin/loadtester-server"]