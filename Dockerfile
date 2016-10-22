FROM postgres:9.5.4

ENV GO_VERSION 1.7.1
ENV GO_HOST_ARCH amd64
ENV GO_HOST_OS linux
ENV GO_ROOT /usr/local/go
ENV GO_DL_BASE https://storage.googleapis.com/golang
ENV GO_DL_URL $GO_DL_BASE/go$GO_VERSION.$GO_HOST_OS-$GO_HOST_ARCH.tar.gz
ENV PATH $GO_ROOT/bin:$PATH

RUN set -x \
    && apt-get update \
    && apt-get install -y curl \
    && rm -rf /var/lib/apt/lists/* \
    && curl $GO_DL_URL | tar xvz -C /usr/local

COPY . /usr/src/stolon

RUN cd /usr/src/stolon \
    && ./build \
    && mv bin/* /usr/local/bin/

COPY run.sh /usr/local/bin

RUN groupadd -r stolon --gid=543 && useradd -r -g stolon --uid=543 stolon

ENTRYPOINT [ "/usr/local/bin/run.sh" ]
