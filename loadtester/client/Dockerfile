FROM golang:1.20.3 AS build-env

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /go/src/github.com/mercari/tortoise/loadtester/client

COPY . .
RUN go build -o ./loadtester-client .

FROM alpine:3.11.6

COPY --from=build-env /go/src/github.com/mercari/tortoise/loadtester/client /loadtester-client
RUN chmod a+x /loadtester-client

CMD ["/loadtester-client"]