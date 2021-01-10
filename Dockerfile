FROM golang as builder

WORKDIR /go/src/app
COPY . .

RUN make build

FROM debian

COPY --from=builder /go/src/app/bin/ /go/src/app/bin/