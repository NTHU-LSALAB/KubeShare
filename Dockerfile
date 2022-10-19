FROM alpine:3.16 AS base

COPY bin/cmd /cmd

CMD ["/cmd"]
