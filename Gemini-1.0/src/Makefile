# Copyright 2020 Hung-Hsin Chen, LSA Lab, National Tsing Hua University
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# path to CUDA installation
CUDA_PATH ?= /usr/local/cuda-10.0
PWD := $(shell pwd)
PREFIX ?= $(PWD)/..

SMS ?=  35 37 50 52 60 61 70
GENCODE_FLAGS += $(foreach sm,$(SMS),-gencode arch=compute_$(sm),code=sm_$(sm))

CXX ?= g++ 
NVCC ?= $(CUDA_PATH)/bin/nvcc -ccbin $(CXX)

CUDA_LDFLAGS += -lcuda -L$(CUDA_PATH)/lib64 -L$(CUDA_PATH)/lib64/stubs 
LDFLAGS += -ldl -lrt 

CXXFLAGS += -std=c++11 -fPIC 

ifeq ($(DEBUG),1)
CXXFLAGS += -g -D_DEBUG -Wall 
else
CXXFLAGS += -O2
endif

# Target rules
all: libgemhook.so.1 gem-schd gem-pmgr

debug.o: debug.cpp debug.h
	g++ -fPIC $(CXXFLAGS) -o $@ -c $<

comm.o: comm.cpp comm.h
	g++ -fPIC $(CXXFLAGS) -o $@ -c $<

hook.o: hook.cpp debug.h comm.h predictor.h util.h
	$(NVCC) -m64 --compiler-options "$(CXXFLAGS)" $(GENCODE_FLAGS) -o $@ -c $<

predictor.o: predictor.cpp predictor.h debug.h
	g++ -fPIC $(CXXFLAGS) -o $@ -c $<

libgemhook.so.1: hook.o predictor.o debug.o comm.o
	$(EXEC) $(NVCC) -shared -m64 $(GENCODE_FLAGS) -o $@ $+ $(CUDA_LDFLAGS) $(LDFLAGS)
	$(EXEC) mkdir -p $(PREFIX)/lib
	$(EXEC) cp $@ $(PREFIX)/lib

scheduler.o: scheduler.cpp debug.h comm.h util.h scheduler.h
	$(EXEC) g++ $(CXXFLAGS) -o $@ -c $<

schd-priority.o: schd-priority.cpp scheduler.h
	$(EXEC) g++ $(CXXFLAGS) -o $@ -c $<

gem-schd: scheduler.o schd-priority.o debug.o comm.o
	$(EXEC) g++ $(LDFLAGS) -pthread -rdynamic  $+ -o $@
	$(EXEC) mkdir -p $(PREFIX)/bin
	$(EXEC) cp $@ $(PREFIX)/bin

pod-manager.o: pod-manager.cpp debug.h comm.h util.h
	$(EXEC) g++ $(CXXFLAGS) -o $@ -c $<

gem-pmgr: pod-manager.o debug.o comm.o
	$(EXEC) g++ $(LDFLAGS) -pthread -rdynamic $+ -o $@
	$(EXEC) mkdir -p $(PREFIX)/bin
	$(EXEC) cp $@ $(PREFIX)/bin

clean:
	rm -f *.o && rm ./gem-schd && rm ./gem-pmgr && rm ./libgemhook.so.1
