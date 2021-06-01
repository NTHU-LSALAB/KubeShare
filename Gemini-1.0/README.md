# Gemini

## About

Gemini is an efficient GPU resource sharing system with fine-grained control for Linux platforms.

It shares a NVIDIA GPU among multiple clients with specified resource constraint, and works seamlessly with any CUDA-based GPU programs. Besides, it is also work-conserving and with low overhead, so nearly no compute resource waste will happen.

## System Structure

Gemini consists of three parts: *scheduler*, *pod manager* and *hook library*.

- *scheduler* (*GPU device manager*) (`gem-schd`): A daemon process managing token. Based on information provided in resource configuration file (`resource-config.txt`), scheduler determines whom to give token. Clients can launch CUDA kernels only when holding a valid token.
- *hook library* (`libgemhook.so.1`): A library intercepting CUDA-related function calls. It utilizes the mechanism of `LD_PRELOAD`, which forces our hook library being loaded before any other dynamic linked libraries.
- *pod manager* (`gem-pmgr`): A proxy for forwarding messages to applications/scheduler. It act as a client to scheduler, and every application sending requests to scheduler via this pod manager shares the token.

Currently we use *TCP socket* as the communication interface between components.

## Build

Basically all components can be built with the following command:

```
make [CUDA_PATH=/path/to/cuda/installation] [PREFIX=/place/to/install] [DEBUG=1]
```

This command will install the built binaries in `$(PREFIX)/bin` and `$(PREFIX)/lib`. Default value for `PREFIX` is `$(pwd)/..`.

Adding `DEBUG=1` in above command will make hook library and executables outputs more scheduling details.

## Usage

### resource configuration file format

First line contains an integer *N*, indicating there are *N* clients.

The following *N* lines are of the format:
```
[ID] [REQUEST] [LIMIT] [GPU_MEM]
```
* `ID`: name of the client (ASCII string less than 63 characters). We use this name as identifier of client, so this name must be unique.
* `REQUEST`: minimum required ratio of GPU usage time (between 0 and 1).
* `LIMIT`: maximum allowed ratio of GPU usage time (between 0 and 1).
* `GPU_MEM`: maximum allowed GPU memory usage (in *bytes*).

Changes to this file will be monitored by `gem-schd`. After each change, scheduler will read this file again and update settings. (\*Note that client must restart to get new memory limit)

### Run

We provide two Python scripts under `tools/` for launching *scheduling system* (`launch-backend.py`) (launches *scheduler* and *pod managers*) and applications (`launch-command.py`).

By default scheduler uses port `50051`, and pod managers use ports starting from `50052`  (`50052`, `50053`, ...).

For more details, refer to those scripts and source code.

## Contributors

[jim90247](https://github.com/jim90247)
[eee4017](https://github.com/eee4017)
[ncy9371](https://github.com/ncy9371)

