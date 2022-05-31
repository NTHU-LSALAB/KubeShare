package scheduler

import (
	"KubeShare/pkg/lib/bitmap"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	// "KubeShare/pkg/lib/stack"
)

const (
	NodeLabelFilter = "SharedGPU"
	// the start port of pod manager
	PodManagerPortStart = 50050
)

// filter the node without label
func (kss *KubeShareScheduler) isGPUNode(obj interface{}) bool {
	node := kss.convertToNode(obj)
	val, ok := node.Labels[NodeLabelFilter]
	if ok && val == "true" {
		return true
	}
	kss.ksl.Warnf("Filter node %v without label SharedGPU=true", node.Name)
	return false
}

func (kss *KubeShareScheduler) addNode(obj interface{}) {
	node := kss.convertToNode(obj)
	name := node.Name

	kss.ksl.Infof("[ADD NODE] %v", name)

	kss.nodePodManagerPortBitmapMutex.Lock()
	defer kss.nodePodManagerPortBitmapMutex.Unlock()

	kss.nodePodManagerPortBitmap[name] = bitmap.NewRRBitmap(512)
	kss.nodePodManagerPortBitmap[name].Mask(0)

	kss.getGPUByNode(name)

	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()

	if isNodeHealth(node) {
		kss.setNodeStatus(name, true)
	} else {
		kss.setNodeStatus(name, false)
	}

}

func (kss *KubeShareScheduler) updateNode(oldObj, newObj interface{}) {
	//oldNode := kss.convertToNode(oldObj)
	newNode := kss.convertToNode(newObj)
	name := newNode.Name
	kss.ksl.Infof("[UPDATE NODE] %v", name)
	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()

	if isNodeHealth(newNode) {
		kss.setNodehealthy(name, true)
	} else {
		kss.setNodehealthy(name, false)
	}
}

func (kss *KubeShareScheduler) deleteNode(obj interface{}) {
	node := kss.convertToNode(obj)
	name := node.Name
	kss.ksl.Infof("[DELETE NODE] %v", name)

	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()
	kss.setNodehealthy(name, false)
}

func (kss *KubeShareScheduler) convertToNode(obj interface{}) *v1.Node {
	switch t := obj.(type) {
	case *v1.Node:
		return obj.(*v1.Node)
	case cache.DeletedFinalStateUnknown:
		if node, ok := t.Obj.(*v1.Node); ok {
			return node
		}
		kss.ksl.Fatalf("Faild to convert DeletedFinalStateUnknown.Obj to Node: %#v", obj)
	default:
		kss.ksl.Fatalf("Faild to convert DeletedFinalStateUnknown.Obj to Node: %#v", obj)
	}
	return nil
}

func isNodeHealth(node *v1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady && c.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
}

// set the cell status according to the specified node status
func (kss *KubeShareScheduler) setNodeStatus(nodeName string, healthy bool) {
	kss.ksl.Debugf("setNodeStatus: %v, %v", nodeName, healthy)

	for _, freeList := range kss.cellFreeList {
		for _, cellList := range freeList {
			for _, cell := range cellList {
				kss.setCellStatus(cell, healthy, nodeName)
			}
		}
	}
}

// set the cell health, uuid, memory
func (kss *KubeShareScheduler) setCellStatus(cell *Cell, healthy bool, nodeName string) {
	// get the gpu information of node & cell type
	GPUs := kss.gpuInfos[nodeName][cell.leafCellType]
	n := len(GPUs)
	// if there is no specified gpu in the node,
	// it means there is no the specified cells that need to be processed too.
	if n == 0 {
		kss.ksl.Debugf("No corresponding gpu %v in the node %v", cell.leafCellType, nodeName)
		return
	}

	s := NewStack()
	s.Push(cell)

	idx := 0

	kss.ksl.Debug("===============CHECK===============")
	kss.ksl.Debugf("Currently, there are %v gpus(%v) in node %v ", n, cell.leafCellType, nodeName)

	for s.Len() > 0 {
		current := s.Pop()

		kss.ksl.Debugf("Cell: %+v", current)

		if current.healthy == healthy {
			continue
		}

		node := current.node
		if node == nodeName || node == "" {
			current.healthy = healthy

			// set the uuid & memory
			if current.level == 1 && idx < n {
				current.uuid = GPUs[idx].uuid
				current.fullMemory = GPUs[idx].memory
				current.freeMemory = current.fullMemory
				idx++
				kss.ksl.Debugf("Level 1: idx: %v, %+v", idx-1, current)
				if current.parent != nil {
					// pass child cells' gpu memory to parent cells
					kss.passMemoryToParent(current)
				}
				kss.leafCellsMutex.Lock()
				kss.leafCells[current.uuid] = current
				kss.ksl.Debugf("Set leaf cell: %v -> %+v", current.uuid, kss.leafCells[current.uuid])
				kss.leafCellsMutex.Unlock()
			}

			parent := current.parent
			if parent != nil && parent.healthy != healthy {
				kss.ksl.Debugf("Parent: %+v", parent)
				s.Push(parent)
			}
			child := current.child
			if child == nil {
				continue
			}
			for i := range child {

				node = child[i].node
				if (node == nodeName || node == "") && child[i].healthy != healthy {
					kss.ksl.Debugf("Child: %+v", child[i])
					s.Push(child[i])
				}
			}

		}
	}

}

// set the cell healthy according to the specified node status
func (kss *KubeShareScheduler) setNodehealthy(nodeName string, healthy bool) {
	kss.ksl.Debugf("setNodehealthy: %v, %v", nodeName, healthy)
	for _, freeList := range kss.cellFreeList {
		for _, cellList := range freeList {
			for _, cell := range cellList {
				kss.setCellHealthy(cell, healthy, nodeName)
			}
		}
	}
}

// set the cell health
func (kss *KubeShareScheduler) setCellHealthy(cell *Cell, healthy bool, nodeName string) {

	s := NewStack()
	s.Push(cell)

	for s.Len() > 0 {
		current := s.Pop()

		kss.ksl.Debugf("Cell: %+v", current)

		if current.healthy == healthy {
			continue
		}

		node := current.node
		if node == nodeName || node == "" {
			current.healthy = healthy

			parent := current.parent
			if parent != nil && parent.healthy != healthy {
				kss.ksl.Debugf("Parent: %+v", parent)
				s.Push(parent)
			}
			child := current.child
			if child == nil {
				continue
			}
			for i := range child {

				node = child[i].node
				if (node == nodeName || node == "") && child[i].healthy != healthy {
					kss.ksl.Debugf("Child: %+v", child[i])
					s.Push(child[i])
				}
			}
		}

	}
}

// pass child cells gpu memory to parent cells
func (kss *KubeShareScheduler) passMemoryToParent(cell *Cell) {
	kss.ksl.Debugf("[passMemoryToParent]")

	s := NewStack()
	s.Push(cell)

	isChild := true
	memory := int64(0)
	for s.Len() > 0 {

		current := s.Pop()
		if isChild {
			isChild = false
			memory = current.fullMemory
			kss.ksl.Debugf("passMemoryToParent - child: %+v", current)
		}

		if current.parent != nil {
			parent := current.parent
			parent.freeMemory += memory
			parent.fullMemory += memory
			if parent.parent != nil {
				s.Push(parent)
			}
			kss.ksl.Debugf("passMemoryToParent -parent: %+v", parent)
		}

	}
}
