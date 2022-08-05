# KubeShare 2.0 Install Document
###### tags: `KubeShare2.0`
## Configuration

### GPU Node
Enable the node with shared GPU setting

```
kubectl label node $nodename SharedGPU=true
```


### GPU Topology 
The cluster manager needs to provide the physical GPU topology.

```shell
vi /kubeshare/scheduler/kubeshare-config.yaml
```

Take the following figure as an example.
![](https://i.imgur.com/mSVcpIZ.png)



1. Config `cellTypes`

A cellType is defined as a type of resource topology.

**Note:**
+ The leaf cell type(the lowest level of `childCellType`) need to be set as GPU model
+ GPU model can see from: `nvidia-smi -L`, please covert the space to '-'
+ The same GPU model need to be set the same priority.
+ We need to mark the node `isNodeLevel:true`

```yaml
cellTypes:
  GTX1080-1NODE:
   childCellType: "NVIDIA-GeForce-GTX-1080"
   childCellNumber: 1
   childCellPriority: 1
   isNodeLevel: true
  GTX1080-2NODE:
    childCellType: "NVIDIA-GeForce-GTX-1080"
    childCellNumber: 2
    isNodeLevel: true
  3-GTX1080-NODE:
    childCellType: GTX1080-2NODE
    childCellNumber: 3
  RTX2070-NODE:
    childCellType: "NVIDIA-GeForce-RTX-2070"
    childCellNumber: 1
    childCellPriority: 2
    isNodeLevel: true
  V100-PCIE-NODE:
    childCellType: "Tesla-V100-PCIE-32GB"
    childCellNumber: 4
    childCellPriority: 100
    isNodeLevel: true
  2-V100-NODE:
    childCellType: V100-PCIE-NODE
    childCellNumber: 2
```

2. Config `cells`

A cell is defined as a resource instance.
```yaml
cells:
- cellType: GTX1080-1NODE
  cellId: apple
- cellType: 3-GTX1080-NODE
  cellChildren:
  - cellId: lemon
  - cellId: orange
  - cellId: grape
- cellType: RTX2070-NODE
  cellId: banana
- cellType: 2-V100-NODE
  cellChildren:
  - cellId: strawberry
  - cellId: strawberry
```
3. Overall

Finally, `kubeshare-config.yaml` would be:

```yaml
cellTypes:
  GTX1080-1NODE:
   childCellType: "NVIDIA-GeForce-GTX-1080"
   childCellNumber: 1
   childCellPriority: 1
   isNodeLevel: true
  GTX1080-2NODE:
    childCellType: "NVIDIA-GeForce-GTX-1080"
    childCellNumber: 2
    isNodeLevel: true
  3-GTX1080-NODE:
    childCellType: GTX1080-2NODE
    childCellNumber: 3
  RTX2070-NODE:
    childCellType: "NVIDIA-GeForce-RTX-2070"
    childCellNumber: 1
    childCellPriority: 2
    isNodeLevel: true
  V100-PCIE-NODE:
    childCellType: "Tesla-V100-PCIE-32GB"
    childCellNumber: 4
    childCellPriority: 100
    isNodeLevel: true
  2-V100-NODE:
    childCellType: V100-PCIE-NODE
    childCellNumber: 2
cells:
- cellType: GTX1080-1NODE
  cellId: apple
- cellType: 3-GTX1080-NODE
  cellChildren:
  - cellId: lemon
  - cellId: orange
  - cellId: grape
- cellType: RTX2070-NODE
  cellId: banana
- cellType: 2-V100-NODE
  cellChildren:
  - cellId: strawberry
  - cellId: strawberry
```

## Installation

1. Install kubeshare-aggregator & kubeshare-collector
```
kubectl apply -f deploy/aggregator.yaml
kubectl apply -f deploy/collector.yaml
```

+ **Make sure the enpoint of kubeshare-aggregator & kubeshare-collector of prometheus is up.**
+ Query the metric `gpu_capacity` , You will get the GPU specification

```
gpu_capacity{endpoint="collector",index="0",instance="xxx.xxx.xxx.xxx:9004",job="kubeshare-collector",memory="34089730048",model="Tesla V100-PCIE-32GB",namespace="kube-system",node="ubuntu",pod="kubeshare-collector-wrrl6",service="kubeshare-collector",uuid="GPU-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"}
```


2. Install node-daemon

In the DaemonSet, modify the `prometheusURL` argument to your prometheus address

```
kubectl apply -f deploy/node-daemon.yaml
```

3. Install scheduler

In the ConfigMap, modify the `prometheusURL` argument to your prometheus address

```
kubectl apply -f deploy/scheduler.yaml
```

## Check

Ensure all the components are running

```
kubectl get pod -A
```
![](https://i.imgur.com/spOp51t.png)
