FROM golang:1.20.3 AS build-env

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
ENV GO111MODULE=on

WORKDIR /go/src/github.com/mercari/tortoise/loadtester/server

COPY . .
RUN go build -o ./loadtester-server .

FROM alpine:3.11.6

COPY --from=build-env /go/src/github.com/mercari/tortoise/loadtester/server /bin/loadtester-server

RUN chmod a+x /bin/loadtester-server \
    && addgroup -S app \
    && adduser -S -G app app \
    && chown app:app /bin/loadtester-server

CMD ["/bin/loadtester-server"]