# KubeShare 2.0
A topology and heterogeneous resource aware scheduler for fractional GPU allocation in Kubernetes cluster  
KubeShare 2.0 is designed in the way of the scheduling framework.

# Features
* Support fractional gpu allocation(<=1) and integer gpu allocation(>1)
* Support GPU Heterogeneity & Topology awareness
* Support Coscheduling


## Prerequisite & Limitation
* A Kubernetes cluster with [garbage collection](https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/), [DNS enabled](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/) [nvidia-continaer-runtime](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)installed.
* Only support a kubernetes cluster that uses the environment variable `NVIDIA_VISIBLE_DEVICES` to control which GPUs will be made accessible inside the container.
* You also ensures that the *prometheus* is installed, because we will pull the data from it.
* It can't compatible with other scheduler to manage gpu resource
* Go version >= v1.16
* Only tested with Kuberenetes v1.18.10


<!--
* GPU attachment setting of container should be going through NVIDIA_VISIBLE_DEVICES environment variable.
-->

## Deployment
1. [Deploy Componments](doc/deploy.md)

## Workloads

### Label description

Because floating point custom device requests is forbidden by K8s, we move GPU resource usage definitions to Labels.
* `sharedgpu/gpu_request` (required if allocating GPU): guaranteed GPU usage of Pod, gpu_request <= "1.0".
* `sharedgpu/gpu_limit` (required if allocating GPU): maximum extra usage if GPU still has free resources, gpu_request <= gpu_limit <= "1.0".
* `sharedgpu/gpu_mem` (optional): maximum GPU memory usage of Pod, in bytes. The default value depends on `gpu_request`
* `sharedgpu/priority`(optional): pod priority 0~100. The default value is 0.
    * priority is equal to 0 represented as an **Opportunistic Pod** used to defragmentation
    * priority is greater than 0 represented as **Guarantee Pod**, which optimizes performance considering locality.
* `sharedgpu/pod_group_name` (optional): the name of pod group for a coscheduling
* `sharedgpu/group_headcount` (optional): the total number of pods in same group
* `sharedgpu/group_threshold` (optional): the minimum proportion of pods to be scheduled together in a pod group.

### Pod specification

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mnist
  labels:
    "sharedgpu/gpu_request": "0.5"
    "sharedgpu/gpu_limit": "1.0"
    "sharedgpu/gpu_model": "NVIDIA-GeForce-GTX-1080"
spec:
  schedulerName: kubeshare-scheduler
  restartPolicy: Never
  containers:
    - name: pytorch
      image:  riyazhu/mnist:test
      command: ["sh", "-c", "sleep infinity"]
      imagePullPolicy: Always #IfNotPresent
```

### Job specification

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: lstm-g
  labels:
    app: lstm-g
spec:
  completions: 5
  parallelism: 5
  template:
    metadata:
      name: lstm-o
      labels:
        "sharedgpu/gpu_request": "0.5"
        "sharedgpu/gpu_limit": "1.0"
        "sharedgpu/group_name": "a"
        "sharedgpu/group_headcount": "5"
        "sharedgpu/group_threshold": "0.2"
        "sharedgpu/priority": "100"
    spec:
      schedulerName: kubeshare-scheduler
      restartPolicy: Never
      containers:
        - name: pytorch
          image:  riyazhu/lstm-wiki2:test
          # command: ["sh", "-c", "sleep infinity"]
          imagePullPolicy: IfNotPresent
          volumeMounts:
          - name: datasets
            mountPath: "/datasets/"
      volumes:
        - name: datasets
          hostPath:
            path: "/home/riya/experiment/datasets/"
```

## Build

### Compiling
```
git clone https://github.com/NTHU-LSALAB/KubeShare.git
cd KubeShare
make
```
* bin/kubeshare-scheduler: schedules pending Pods to node and device, i.e. <nodeName, GPU UUID>.
* bin/kubeshare-collector: collect the GPU specification
* bin/kubeshare-aggregator(gpu register): register pod GPU requirement.
* bin/kubeshare-config: update the config file for Gemini
* bin/kubeshare-query-ip: inject current node ip for Gemini


### Build & Push images 
```
make build-image
make push-image
```
* chanage variables `CONTAINER_PREFIX`, `CONTAINER_NAME`, `CONTAINER_VERSION`

### Directories & Files
* cmd/: where main function located of three binaries.
* docker/: materials of all docker images in yaml files.
* pkg/: includes KubeShare 2.0 core components.
* deploy/: the install yaml files.
* go.mod: KubeShare dependencies.

## GPU Isolation Library
Please refer to [Gemini](https://github.com/NTHU-LSALAB/Gemini).

# TODO
* Optimize the locality function.  
* Modify the prometheus to etcd.
* Automatically detect GPU topology.

# Issues
Any issues please open a GitHub issue, thanks.


