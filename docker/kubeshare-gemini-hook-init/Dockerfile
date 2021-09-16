# call from repo root

FROM nvidia/cuda:10.0-devel AS build

WORKDIR /tmp/build



COPY ./Gemini Gemini

RUN cd Gemini && \
    make -C src CUDA_PATH=/usr/local/cuda/ DEBUG=1

# RUN apt update && \
#     apt install -y git && \
#     git clone https://github.com/NTHU-LSALAB/Gemini.git && cd Gemini && \
#     git reset --hard cf199415bbc8d0ef8fa97c917e6d7cdd200fdfd0 && \
#     make -C src CUDA_PATH=/usr/local/cuda/
    #make -C src
    # CMD ["sleep", "infinity"]

# FROM busybox:1.31.1-glibc
FROM alpine:3.9

COPY --from=build /tmp/build/Gemini/lib/libgemhook.so.1 /libgemhook.so.1
CMD ["cp", "/libgemhook.so.1", "/kubeshare/library/libgemhook.so.1"]
