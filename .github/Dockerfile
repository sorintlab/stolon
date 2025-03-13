ARG PGVERSION=0
ARG ARCHITECTURE=amd64

#######
####### Build the stolon binaries
#######
FROM golang:1.24-bookworm AS builder

WORKDIR /stolon

# copy all the sources
COPY . .

RUN make

#######
####### Build the final image
#######
FROM postgres:${PGVERSION}

RUN useradd -ms /bin/bash stolon

EXPOSE 5432

COPY --from=builder /stolon/bin/ /usr/local/bin

RUN chmod +x /usr/local/bin/stolon-keeper /usr/local/bin/stolon-sentinel /usr/local/bin/stolon-proxy /usr/local/bin/stolonctl
