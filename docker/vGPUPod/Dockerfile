# call from repo root

FROM nvidia/cuda:10.0-devel AS build

WORKDIR /tmp/build

COPY . .

RUN apt update && \
    apt install -y build-essential && \
    cd docker/vGPUPod && \
    make

FROM busybox:1.31.1-glibc

ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

COPY docker/lib/libdl.so.2          /lib
COPY docker/lib/libnvidia-ml.so.1   /lib

COPY --from=build /tmp/build/docker/vGPUPod/uuid /usr/bin/print-uuid

CMD ["print-uuid"]
