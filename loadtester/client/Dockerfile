FROM golang:1.20.3 AS build-env

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /go/src/github.com/mercari/tortoise/loadtester/client

COPY . .
RUN go build -o ./loadtester-client .

FROM alpine:latest

COPY --from=build-env /go/src/github.com/mercari/tortoise/loadtester/client/loadtester-client /bin/loadtester-client

RUN apk --no-cache add ca-certificates

RUN addgroup -g 1001 app && adduser -D -G app -u 1001 app

USER 1001

CMD ["/bin/loadtester-client"]