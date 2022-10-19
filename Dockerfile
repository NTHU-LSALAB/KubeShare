FROM alpine:3.16 AS base

COPY bin/kubeshare /kubeshare

CMD ["/kubeshare"]
