# call from repo root

FROM ubuntu:18.04 AS build

ENV GOLANG_VERSION 1.13.5
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /go/src/kubeshare

COPY . .

RUN apt update && \
    apt install -y g++ wget make && \
    wget -nv -O - https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz && \
    make kubeshare-device-manager

FROM alpine:3.9

COPY --from=build /go/src/kubeshare/bin/kubeshare-device-manager /usr/bin/kubeshare-device-manager

CMD ["kubeshare-device-manager", "-alsologtostderr", "-v=4"]
