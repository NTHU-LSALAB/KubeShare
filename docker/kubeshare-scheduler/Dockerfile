# call from repo root

FROM golang:1.17 AS build

WORKDIR /go/src/kubeshare

COPY . .

RUN make kubeshare-scheduler

FROM debian:stretch-slim

COPY --from=build /go/src/kubeshare/bin/kubeshare-scheduler /usr/bin/kubeshare-scheduler

CMD ["kubeshare-scheduler", "-alsologtostderr", "-v=4"]

# docker cp . nifty_maxwell:/go

# export GOLANG_VERSION=1.13.5
# export GOPATH=/go
# export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH
# mkdir -p /go/src/KubeShare
# cd /go/src/KubeShare
