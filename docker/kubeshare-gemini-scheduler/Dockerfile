# call from repo root

FROM nvidia/cuda:10.0-devel AS build

WORKDIR /tmp/build

COPY . kubeshare

# RUN apt update && \
#     apt install -y git && \
#     git clone https://github.com/NTHU-LSALAB/Gemini.git && cd Gemini && \
#     make -C src

COPY ./Gemini Gemini

RUN cd Gemini && \
    make -C src CUDA_PATH=/usr/local/cuda/ DEBUG=1


FROM python:3.8.1-buster

RUN pip3 install inotify

ENV NVIDIA_VISIBLE_DEVICES      all
ENV NVIDIA_DRIVER_CAPABILITIES  utility

COPY docker/lib/libdl.so.2          /lib
COPY docker/lib/libnvidia-ml.so.1   /lib

COPY --from=build /tmp/build/kubeshare/docker/kubeshare-gemini-scheduler/launcher-multigpus.sh /launcher-multigpus.sh
COPY --from=build /tmp/build/kubeshare/docker/kubeshare-gemini-scheduler/launcher.py /launcher.py
COPY --from=build /tmp/build/Gemini/bin/gem-schd /gem-schd
COPY --from=build /tmp/build/Gemini/bin/gem-pmgr /gem-pmgr
#COPY --from=build /tmp/build/kubeshare/docker/kubeshare-gemini-scheduler/launch-backend.py /launch-backend.py
CMD ["/launcher-multigpus.sh", "/kubeshare/scheduler/config", "/kubeshare/scheduler/podmanagerport"]
#CMD ["/launch-backend.py", "/gem-schd", "/gem-pmgr"]
