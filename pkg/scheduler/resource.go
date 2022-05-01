package scheduler

/** Filter **/
// filter node according to the node and its gpu model
func (kss *KubeShareScheduler) filterNode(nodeName, model string, request float64, memory int64) (bool, float64, int64) {
	kss.ksl.Debugf("filterNode: node %v with gpu model %v", nodeName, model)

	ok := false
	available := 0.0
	freeMemory := int64(0)
	freeList := kss.cellFreeList[model]
	for _, cellList := range freeList {
		for _, cell := range cellList {
			fit, currentAvailable, currentMemory := kss.checkCellResource(cell, nodeName, request, memory)
			ok = ok || fit
			available += currentAvailable
			freeMemory += currentMemory

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

	for s.Len() > 0 {
		current := s.Pop()
		kss.ksl.Debugf("Check resource cell: %+v", current)

		if current.node == nodeName && current.healthy {
			// only need whole gpu
			if multiGPU {
				availableWholeCell := current.availableWholeCell
				freeMemory := current.freeMemory
				if availableWholeCell >= request && freeMemory >= memory {
					return true, availableWholeCell, freeMemory
				} else {
					return false, availableWholeCell, freeMemory
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
				kss.ksl.Debugf("Check resource child: %+v", child[i])
				s.Push(child[i])
			}
		}
	}
	return false, 0, 0
}

/*
func (kss *KubeShareScheduler) checkCellResource(cell *Cell, nodeName string, request float64, memory int64) (bool, float64, int64) {
	s := NewStack()

	node := cell.node
	// this cell does not need to check
	if node != nodeName && node != "" {
		return false, 0.0, 0
	}

	if node == "" || cell.healthy {
		s.Push(cell)
	}

	// store the number of whole gpu
	available := 0.0
	freeMemory := int64(0)
	multiGPU := request > 1.0

	for s.Len() > 0 {
		current := s.Pop()
		kss.ksl.Debugf("Check resource cell: %+v", current)

		if current.level == 1 {

			if multiGPU {
				if current.available == 1.0 {
					available += 1.0
					freeMemory += current.freeMemory

					if available >= request && freeMemory >= memory {
						return true, available, freeMemory
					}
				}
			} else {
				if current.available >= request && current.freeMemory >= memory {
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
				kss.ksl.Debugf("Check resource child: %+v", child[i])
				s.Push(child[i])
			}
		}
	}
	return false, available, freeMemory
}
*/
