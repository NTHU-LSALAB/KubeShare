# call from repo root

FROM golang:1.15.7 AS build


WORKDIR /go/src/kubeshare

COPY . .

RUN make kubeshare-query-ip

FROM alpine:3.9

COPY --from=build /go/src/kubeshare/bin/kubeshare-query-ip /usr/bin/kubeshare-query-ip

CMD ["kubeshare-query-ip"]



