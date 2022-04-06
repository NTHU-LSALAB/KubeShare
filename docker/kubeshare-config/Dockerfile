# call from repo root

FROM ubuntu:18.04 AS build

ENV GOLANG_VERSION 1.15.7
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /go/src/kubeshare

COPY . .

RUN apt update && \
    apt install -y g++ wget make && \
    wget -nv -O - https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz && \
    make kubeshare-config

FROM debian:stretch-slim

ENV NVIDIA_VISIBLE_DEVICES      all
ENV NVIDIA_DRIVER_CAPABILITIES  utility

COPY --from=build /go/src/kubeshare/bin/kubeshare-config /usr/bin/kubeshare-config

CMD ["kubeshare-config"]
