package scheduler

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

/** for regular pod **/
// kubeshare treats gpu resource as rare resource
// if the node without gpu, node score will be set to 100
// otherwise, node score will be set to be 0
func (kss *KubeShareScheduler) calculateRegularPodNodeScore(nodeName string) float64 {

	if len(kss.gpuInfos[nodeName]) > 0 {
		return 100
	}

	return 0
}

/** for opportunistic pod **/
func (kss *KubeShareScheduler) calculateOpportunisticPodScore(nodeName string, podStatus *PodStatus) float64 {

	model := podStatus.model
	score := 0.0
	// assigned gpu model
	if model != "" {
		score = kss.calculateOpportunisticPodNodeScore(kss.getModelLeafCellbyNode(nodeName, model))

	} else {
		//get the gpu information in the node
		score = kss.calculateOpportunisticPodNodeScore(kss.getAllLeafCellbyNode(nodeName))
	}
	return score
}

// score = ( cell priority(computation power)
//           + gpu resource usage(defragmentation)
//           - # of free leaf cell (defragmentation)(%) ) / # of cell
func (kss *KubeShareScheduler) calculateOpportunisticPodNodeScore(cellList CellList) float64 {
	if cellList == nil {
		return 0
	}
	// number of free leaf cells
	freeLeafCell := 0.0
	score := 0.0
	for _, cell := range cellList {
		//
		score += float64(kss.gpuPriority[cell.cellType])
		// gpu resource
		available := cell.available
		if available == 1 {
			freeLeafCell += 1
			// gpu usage : 0
		} else {
			// gpu usage
			score += (1 - cell.available) * 100
		}
		kss.ksl.Debugf("OpportunisticPodNodeScore %v with score: %v, free leaf cell %v", cell.cellType, score, freeLeafCell)
	}

	n := float64(len(cellList))
	score -= freeLeafCell / n * 100
	kss.ksl.Debugf("OpportunisticPodNodeScore with freeLeaf score: %v", score)
	return score / n
}

/** for guarantee pod **/
func (kss *KubeShareScheduler) calculateGuaranteePodScore(nodeName string, podStatus *PodStatus) float64 {
	model := podStatus.model
	score := 0.0
	if model != "" {
		score = kss.calculateGuaranteePodNodeScore(kss.getModelLeafCellbyNode(nodeName, model), podStatus.podGroup)
	} else {
		score = kss.calculateGuaranteePodNodeScore(kss.getAllLeafCellbyNode(nodeName), podStatus.podGroup)
	}
	return score
}

// socre = ( cell priority(computation power)
//          -  gpu resource usage(defragmentation)
//          -  average locality(placement sensitivity) ) / # of cell
func (kss *KubeShareScheduler) calculateGuaranteePodNodeScore(cellList CellList, podGroup string) float64 {
	if cellList == nil {
		return 0
	}

	score := 0.0
	idGroup := kss.filterPodGroup(podGroup) // kss.getCellIDFromPodGroup(podGroup)
	nGroup := float64(len(idGroup))

	for _, cell := range cellList {
		score += float64(kss.gpuPriority[cell.cellType]) - ((1 - cell.available) * 100)
		kss.ksl.Debugf("GuaranteePodNodeScore %v(%v) with score: %v", cell.cellType, cell.id, score)
		if nGroup == 0 {
			kss.ksl.Debugf("No pod in same group, jump ...")
			continue
		}
		locality := 0.0
		currentId := strings.Split(cell.id, "/")
		for _, id := range idGroup {
			dis := kss.getCellIDDistance(currentId, id)
			locality += dis
			kss.ksl.Debugf("distance %v <-> %v: %v", cell.id, id, dis)
		}
		score -= locality / nGroup * 100
		kss.ksl.Debugf("GuaranteePodNodeScore %v(%v) with Locality score: %v", cell.cellType, cell.id, score)
	}
	return score / float64(len(cellList))
}

/*
func (kss *KubeShareScheduler) queryPodFromPodGroup(podGroup string) []model.LabelSet {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, warnings, err := kss.promeAPI.Series(ctx, []string{
		"{__name__=~\"" + GPU_REQUIREMENT + "\",group_name=\"" + podGroup + "\"}",
	}, time.Now().Add(-time.Second*5), time.Now())

	if err != nil {
		kss.ksl.Warnf("Error querying Prometheus: %v\n", err)
		return nil
	}
	if len(warnings) > 0 {
		kss.ksl.Warnf("Warnings: %v\n", warnings)
	}
	kss.ksl.Debugf("[queryPodFromPodGroup] %+v: %v", podGroup, len(result))
	return result
}

func (kss *KubeShareScheduler) getCellIDFromPodGroup(podGroup string) []string {
	results := kss.queryPodFromPodGroup(podGroup)
	kss.ksl.Debugf("[getCellIDFromPodGroup] %+v: %v", podGroup, len(results))
	cellIDList := []string{}

	for _, res := range results {
		id := string(res["cell_id"])
		if id != "" {
			cellIDList = append(cellIDList, id)
		}

	}
	return cellIDList
}
*/

func (kss *KubeShareScheduler) filterPodGroup(podGroup string) []string {
	cellIDList := []string{}
	for _, ps := range kss.podStatus {
		if ps.podGroup == podGroup {
			cells := ps.cells
			for _, cell := range cells {
				cellIDList = append(cellIDList, cell.id)
			}
		}
	}
	kss.ksl.Debugf("[filterPodGroup] %+v: %v", podGroup, len(cellIDList))
	return cellIDList
}

func (kss *KubeShareScheduler) getCellIDDistance(current []string, id string) float64 {
	idList := strings.Split(id, "/")

	nCurrent := len(current)
	nID := len(idList)
	distance := 0.0
	if nCurrent >= nID {
		i, j := nID-1, nCurrent-1

		for i >= 0 {
			cur, err1 := strconv.Atoi(current[j])
			iid, err2 := strconv.Atoi(idList[i])
			if err1 != nil || err2 != nil {
				tmpC, tmpI := current[j], idList[i]
				kss.ksl.Warnf("[parseCellID] covert node name-> %v vs %v", tmpC, tmpI)
				if tmpC != tmpI {
					distance += 100
				}
			} else {
				distance += math.Abs(float64(cur) - float64(iid))
			}
			i--
			j--
		}
		for ; j >= 0; j-- {
			cur, err1 := strconv.Atoi(current[j])
			if err1 != nil {
				kss.ksl.Errorf("[parseCellID] convert error: %v", err1)
				distance += 100
			} else {
				distance += float64(cur)
			}
		}

	} else {
		i, j := nID-1, nCurrent-1
		for j >= 0 {
			cur, err1 := strconv.Atoi(current[j])
			iid, err2 := strconv.Atoi(idList[i])
			if err1 != nil || err2 != nil {
				tmpC, tmpI := current[j], idList[i]
				kss.ksl.Warnf("[parseCellID] covert node name-> %v vs %v", tmpC, tmpI)
				if tmpC != tmpI {
					distance += 100
				}
			} else {
				distance += math.Abs(float64(cur) - float64(iid))
			}
			i--
			j--
		}
		for ; i >= 0; i-- {
			iid, err2 := strconv.Atoi(idList[i])
			if err2 != nil {
				kss.ksl.Errorf("[parseCellID] convert error: %v", err2)
				distance += 100
			} else {
				distance += float64(iid)
			}
		}
	}

	return distance
}

// get the all leaf cell according to node's model
func (kss *KubeShareScheduler) getModelLeafCellbyNode(nodeName, model string) CellList {

	var cl CellList
	freeList := kss.cellFreeList[model]
	for _, cellList := range freeList {
		for _, cell := range cellList {
			cl = append(cl, kss.getLeafCellbyNode(cell, nodeName)...)
		}
	}
	return cl
}

// get all leaf cell according to the node
func (kss *KubeShareScheduler) getAllLeafCellbyNode(nodeName string) CellList {

	var cl CellList
	for _, freeList := range kss.cellFreeList {
		for _, cellList := range freeList {
			for _, cell := range cellList {
				cl = append(cl, kss.getLeafCellbyNode(cell, nodeName)...)
			}
		}
	}
	return cl
}

// get the leaf cell
func (kss *KubeShareScheduler) getLeafCellbyNode(cell *Cell, nodeName string) CellList {

	node := cell.node

	if node != nodeName && node != "" {
		return nil
	}

	s := NewStack()
	var cellList CellList

	if cell.healthy {
		s.Push(cell)
	}

	for s.Len() > 0 {
		current := s.Pop()
		if current.level == 1 {
			kss.ksl.Debugf("[getLeafCellbyNode] %v => %+v", nodeName, current)
			cellList = append(cellList, current)
		}

		node = current.node
		if node == nodeName || node == "" {
			child := current.child
			if child == nil {
				continue
			}
			for i := range child {
				if (node == nodeName || node == "") && child[i].healthy {

					s.Push(child[i])
				}
			}
		}
	}
	return cellList
}

/*for Reserve*/
func (kss *KubeShareScheduler) calculateOpportunisticPodCellScore(nodeName string, podStatus *PodStatus) CellList {
	model := podStatus.model
	// get cell list
	var cellList CellList

	finalList := CellList{}

	request := podStatus.request
	memory := podStatus.memory
	multiGPU := request > 1.0

	type CellScore struct {
		cell  *Cell
		score float64
	}
	scores := []*CellScore{}

	if model != "" {
		cellList = kss.getModelLeafCellbyNode(nodeName, model)
	} else {
		cellList = kss.getAllLeafCellbyNode(nodeName)
	}

	if multiGPU {
		for _, cell := range cellList {
			if cell.available == 1 {
				scores = append(scores, &CellScore{cell: cell, score: float64(cell.priority)})
			}
		}
	} else {
		for _, cell := range cellList {
			score := float64(cell.priority) + ((1 - cell.available) * 100)
			scores = append(scores, &CellScore{cell: cell, score: score})
		}
	}

	sort.Slice(scores, func(p, q int) bool {
		return scores[p].score > scores[q].score
	})

	for _, cs := range scores {
		cell := cs.cell
		if multiGPU {
			finalList = append(finalList, cell)
			request -= 1.0
		} else {
			if cell.available >= request && cell.freeMemory >= memory {
				finalList = append(finalList, cell)
				request = 0
			}
		}

		if request == 0 {
			break
		}
	}
	return finalList
}

func (kss *KubeShareScheduler) calculateGuaranteePodCellScore(nodeName string, podStatus *PodStatus) CellList {
	model := podStatus.model
	// get cell list
	var cellList CellList

	finalList := CellList{}

	request := podStatus.request
	memory := podStatus.memory
	multiGPU := request > 1.0

	type CellScore struct {
		cell  *Cell
		score float64
	}
	scores := []*CellScore{}

	if model != "" {
		cellList = kss.getModelLeafCellbyNode(nodeName, model)
	} else {
		cellList = kss.getAllLeafCellbyNode(nodeName)
	}

	idGroup := kss.filterPodGroup(podStatus.podGroup) // kss.getCellIDFromPodGroup(podStatus.podGroup)
	nGroup := float64(len(idGroup))

	if multiGPU {
		for _, cell := range cellList {
			if cell.available == 1 {
				score := float64(cell.priority)
				if nGroup > 0 {
					locality := 0.0
					currentId := strings.Split(cell.id, "/")
					for _, id := range idGroup {
						dis := kss.getCellIDDistance(currentId, id)
						locality += dis
						kss.ksl.Debugf("[Cell] distance %v <-> %v: %v", cell.id, id, dis)
					}
					score -= locality / nGroup * 100
				}
				scores = append(scores, &CellScore{cell: cell, score: score})
			}
		}
	} else {
		for _, cell := range cellList {
			score := float64(cell.priority) - ((1 - cell.available) * 100)
			if nGroup > 0 {
				locality := 0.0
				currentId := strings.Split(cell.id, "/")
				for _, id := range idGroup {
					dis := kss.getCellIDDistance(currentId, id)
					locality += dis
					kss.ksl.Debugf("[Cell] distance %v <-> %v: %v", cell.id, id, dis)
				}
				score -= ((locality / nGroup) * 100)
			}
			scores = append(scores, &CellScore{cell: cell, score: score})
		}
	}

	sort.Slice(scores, func(p, q int) bool {
		return scores[p].score > scores[q].score
	})

	for _, cs := range scores {
		cell := cs.cell
		if multiGPU {
			finalList = append(finalList, cell)
			request -= 1.0
		} else {
			if cell.available >= request && cell.freeMemory >= memory {
				finalList = append(finalList, cell)
				request = 0
			}
		}

		if request == 0 {
			break
		}
	}
	return finalList

}
