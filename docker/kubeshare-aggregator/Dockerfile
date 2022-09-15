# call from repo root

FROM golang:1.17 AS build

WORKDIR /go/src/kubeshare

COPY . .

RUN make kubeshare-aggregator

FROM debian:stretch-slim

ENV NVIDIA_VISIBLE_DEVICES      all
ENV NVIDIA_DRIVER_CAPABILITIES  utility

COPY --from=build /go/src/kubeshare/bin/kubeshare-aggregator /usr/bin/kubeshare-aggregator

CMD ["kubeshare-aggregator"]
