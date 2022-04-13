package scheduler

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"KubeShare/pkg/lib/stack"
)

const (
	NodeLabelFilter = "SharedGPU"
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

	kss.ksl.Infof("[New node Event] %v", name)

	kss.cellMutex.Lock()
	defer kss.cellMutex.Unlock()

	if isNodeHealth(node) {
		kss.setNodeStatus(name, true)
	} else {

	}

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

// marks a node and the cells in it as healthy or bad
func (kss *KubeShareScheduler) setNodeStatus(nodeName string, healthy bool) {
	kss.ksl.Debugf("setNodeStatus: %v, %v", nodeName, healthy)

	for _, freeList := range kss.cellFreeList {
		for _, cellList := range freeList {
			for _, cell := range cellList {
				kss.setCellHeathy(cell, healthy, nodeName)
			}
		}
	}

	kss.ksl.Debugln("=================FREE CELL=================")
	for k, v := range kss.cellFreeList {
		kss.ksl.Debugf("%+v = %+v", k, v)
	}
}

//TODO: set uuid
func (kss *KubeShareScheduler) setCellHeathy(cell *Cell, healthy bool, nodeName string) {
	s := stack.NewStack()
	s.Push(cell)

	for s.Len() > 0 {
		current := s.Pop().(*Cell)
		kss.ksl.Debugf("Cell: %+v", current)
		if current.healthy == healthy {
			continue
		}
		node := current.node
		if node == nodeName || node == "" {
			current.healthy = healthy
			parent := current.parent
			if parent != nil && parent.healthy != healthy {
				kss.ksl.Debugf("Parent: %+v", current)
				s.Push(parent)
			}
			child := current.child
			if child == nil {
				continue
			}
			for i := range child {

				node = child[i].node
				if (node == nodeName || node == "") && child[i].healthy != healthy {
					kss.ksl.Debugf("Child: %+v", current)
					s.Push(child[i])
				}
			}
		}

	}
}
