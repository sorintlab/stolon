FROM golang

WORKDIR /go/src/app
COPY . .

RUN make build