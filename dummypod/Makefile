PROGRAMS = uuid

CUDA_PATH ?= /usr/local/cuda

.PHONY: all clean

uuid: uuid.c
	gcc uuid.c -o uuid -I $(CUDA_PATH)/include/ -L $(CUDA_PATH)/lib64/ -lnvidia-ml

clean:
	rm $(PROGRAMS)
