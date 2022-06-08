package scheduler

/** Filter **/
// filter node according to the node and its gpu model
func (kss *KubeShareScheduler) filterNode(nodeName, model string, request float64, memory int64) (bool, float64, int64) {
	kss.ksl.Debugf("[Filter] filterNode: node %v with gpu model %v", nodeName, model)

	ok := false
	available := 0.0
	freeMemory := int64(0)
	freeList := kss.cellFreeList[model]

	for _, cellList := range freeList {
		for _, cell := range cellList {
			kss.ksl.Debugf("[Filter] filterNode: cell %v", cell)
			fit, currentAvailable, currentMemory := kss.checkCellResource(cell, nodeName, request, memory)
			ok = ok || fit
			available += currentAvailable
			freeMemory += currentMemory
			// prune
			if ok {
				return ok, available, freeMemory
			}
		}
	}

	return ok, available, freeMemory
}

// check if the gpu resource in the node can fit the pod requirement
// and calculate its free resource in the specified gpu model
func (kss *KubeShareScheduler) checkCellResource(cell *Cell, nodeName string, request float64, memory int64) (bool, float64, int64) {
	s := NewStack()

	node := cell.node
	// this cell does not need to check
	if node != nodeName && node != "" {
		return false, 0.0, 0
	}

	if cell.healthy {
		s.Push(cell)
	}

	multiGPU := request > 1.0
	availableWholeCell := float64(0.0)
	freeMemory := int64(0)

	if multiGPU {
		for s.Len() > 0 {
			current := s.Pop()
			kss.ksl.Debugf("[Filter] Check cell resource (multiGPU): %+v", current)

			if current.node == nodeName && current.isNode && current.healthy {
				availableWholeCell += current.availableWholeCell
				freeMemory += current.freeMemory
				kss.ksl.Debugf("[Filter] node %v, availableWholeCell %v request-> %v, freeMemory %v reqeust-> %v", nodeName, availableWholeCell, request, freeMemory, memory)
				if availableWholeCell >= request && freeMemory >= memory {
					return true, availableWholeCell, freeMemory
				}
			}

			child := current.child
			if child == nil {
				continue
			}
			if current.higherThanNode && current.healthy {
				for i := range child {
					node := child[i].node
					if (node == nodeName || node == "") && child[i].healthy {
						kss.ksl.Debugf("[Filter] Check child resource (multiGPU): %+v", child[i])
						s.Push(child[i])
					}
				}
			}
		}
	} else {
		for s.Len() > 0 {
			current := s.Pop()
			kss.ksl.Debugf("[Filter] Check cell resource (sharedGPU): %+v", current)
			if current.node == nodeName && current.healthy && current.level == 1 {
				if current.available >= request && current.freeMemory >= memory {
					return true, current.available, current.freeMemory
				}
			}
			child := current.child
			if child == nil {
				continue
			}
			for i := range child {
				node := child[i].node
				if (node == nodeName || node == "") && child[i].healthy {
					kss.ksl.Debugf("[Filter] Check child resource (sharedGPU): %+v", child[i])
					s.Push(child[i])
				}
			}
		}
	}
	if multiGPU {
		return false, availableWholeCell, freeMemory
	} else {
		return false, 0, 0
	}
}

/*
// filter node according to the node and its gpu model
func (kss *KubeShareScheduler) filterNode(nodeName, model string, request float64, memory int64) (bool, float64, int64) {
	kss.ksl.Debugf("[Filter] filterNode: node %v with gpu model %v", nodeName, model)

	ok := false
	available := 0.0
	freeMemory := int64(0)
	freeList := kss.cellFreeList[model]

	for _, cellList := range freeList {
		for _, cell := range cellList {
			kss.ksl.Debugf("[Filter] filterNode: cell %v", cell)
			fit, currentAvailable, currentMemory := kss.checkCellResource(cell, nodeName, request, memory)
			ok = ok || fit
			available += currentAvailable
			freeMemory += currentMemory
			// prune
			if ok {
				return ok, available, freeMemory
			}
		}
	}

	return ok, available, freeMemory
}

func (kss *KubeShareScheduler) checkCellResource(cell *Cell, nodeName string, request float64, memory int64) (bool, float64, int64) {
	s := NewStack()

	node := cell.node
	// this cell does not need to check
	if node != nodeName && node != "" {
		return false, 0.0, 0
	}

	if cell.healthy {
		s.Push(cell)
	}

	multiGPU := request > 1.0
	availableWholeCell := float64(0.0)
	freeMemory := int64(0)
	for s.Len() > 0 {
		current := s.Pop()
		kss.ksl.Debugf("[Filter] Check resource cell: %+v", current)

		if current.node == nodeName && current.healthy {
			// only need whole gpu
			if multiGPU {
				availableWholeCell += current.availableWholeCell
				freeMemory += current.freeMemory
				kss.ksl.Debugf("[Filter] node %v, availableWholeCell %v request-> %v, freeMemory %v reqeust-> %v", nodeName, availableWholeCell, request, freeMemory, memory)
				if availableWholeCell >= request && freeMemory >= memory {
					return true, availableWholeCell, freeMemory
				}
			} else {
				if current.level == 1 && current.available >= request && current.freeMemory >= memory {
					return true, current.available, current.freeMemory
				}
			}
		}

		child := current.child
		if child == nil {
			continue
		}

		for i := range child {
			node := child[i].node
			if (node == nodeName || node == "") && child[i].healthy {
				kss.ksl.Debugf("[Filter] Check resource child: %+v", child[i])
				s.Push(child[i])
			}
		}
	}

	if multiGPU {
		return false, availableWholeCell, freeMemory
	} else {
		return false, 0, 0
	}

}
*/
