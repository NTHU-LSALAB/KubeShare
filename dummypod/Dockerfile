FROM busybox:1.31.1-glibc

ADD libdl.so.2 /lib
ADD libnvidia-ml.so.1 /lib
ADD uuid /

ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

CMD ["sh", "-c", "/uuid; while true; do sleep 3650d; done"]
