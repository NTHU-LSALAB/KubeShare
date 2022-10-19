FROM debian:stretch-slim

ENV NVIDIA_VISIBLE_DEVICES      all
ENV NVIDIA_DRIVER_CAPABILITIES  utility

COPY bin/kubeshare /kubeshare

CMD ["/kubeshare"]
