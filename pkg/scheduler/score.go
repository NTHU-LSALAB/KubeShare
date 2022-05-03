package scheduler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

/** for regular pod **/
// kubeshare treats gpu resource as rare resource
// if the node without gpu, node score will be set to 100
// otherwise, node score will be set to be 0
func (kss *KubeShareScheduler) calculateRegularPodNodeScore(nodeName string) int64 {

	if len(kss.gpuInfos[nodeName]) > 0 {
		return int64(100)
	}

	return int64(0)
}

/** for opportunistic pod **/
func (kss *KubeShareScheduler) calculateOpportunisticPodScore(nodeName string, podStatus *PodStatus) int64 {

	model := podStatus.model
	score := int64(0)
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
func (kss *KubeShareScheduler) calculateOpportunisticPodNodeScore(cellList CellList) int64 {
	if cellList == nil {
		return 0
	}
	// number of free leaf cells
	freeLeafCell := int64(0)
	score := int64(0)
	for _, cell := range cellList {
		//
		score += int64(kss.gpuPriority[cell.cellType])
		// gpu resource
		available := cell.available
		if available == 1 {
			freeLeafCell += 1
			// gpu usage : 0
		} else {
			// gpu usage
			score += int64((1 - cell.available) * 100)
		}
		kss.ksl.Debugf("OpportunisticPodNodeScore %v with score: %v", cell.cellType, score)
	}

	n := int64(len(cellList))
	score -= int64(freeLeafCell / n * 100) //
	return int64(score / n)
}

/** for guarantee pod **/
func (kss *KubeShareScheduler) calculateGuaranteePodScore(nodeName string, podStatus *PodStatus) int64 {
	model := podStatus.model
	score := int64(0)
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
func (kss *KubeShareScheduler) calculateGuaranteePodNodeScore(cellList CellList, podGroup string) int64 {
	if cellList == nil {
		return 0
	}

	score := int64(0)
	stringID := kss.getCellIDFromPodGroup(podGroup)
	cellIDList := kss.convertCellID(stringID)
	nGroup := len(cellIDList)

	for _, cell := range cellList {
		score += int64(kss.gpuPriority[cell.cellType]) - int64((1-cell.available)*100)
		kss.ksl.Debugf("GuaranteePodNodeScore %v with score: %v", cell.cellType, score)
		if nGroup == 0 {
			kss.ksl.Debugf("No pod in same group, jump ...")
			continue
		}
		locality := 0
		currentId := kss.parseCellID(cell.id)
		for i, id := range cellIDList {
			dis := id - currentId
			if dis < 0 {
				dis *= -1
			}
			locality += dis
			kss.ksl.Debugf("distance %v <-> %v: %v", cell.id, stringID[i], dis)
		}
		score += int64(locality / nGroup)
	}
	return score / int64(len(cellList))
}

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
	return result
}

func (kss *KubeShareScheduler) getCellIDFromPodGroup(podGroup string) []string {
	results := kss.queryPodFromPodGroup(podGroup)

	cellIDList := []string{}

	for _, res := range results {
		cellID := strings.Split(string(res["cell_id"]), ",")
		for _, id := range cellID {
			if id != "" {
				cellIDList = append(cellIDList, id)
			}
		}
	}
	return cellIDList
}

func (kss *KubeShareScheduler) parseCellID(id string) int {
	idList := strings.Split(id, "\\")
	nID := len(idList)
	last := idList[nID-1]

	num, err := strconv.Atoi(last)
	if err != nil {
		kss.ksl.Errorf("[parseCellID] convert error: %v", err)
	}

	return num
}

func (kss *KubeShareScheduler) convertCellID(cellIDList []string) []int {
	idList := make([]int, len(cellIDList))
	for i, id := range cellIDList {
		idList[i] = kss.parseCellID(id)
	}
	return idList
}

// get the all leaf cell according to node's model
func (kss *KubeShareScheduler) getModelLeafCellbyNode(nodeName, model string) CellList {

	var cl CellList
	freeList := kss.cellFreeList[model]
	for _, cellList := range freeList {
		for _, cell := range cellList {
			cl = appendCellList(cl, kss.getLeafCellbyNode(cell, nodeName))
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
				cl = appendCellList(cl, kss.getLeafCellbyNode(cell, nodeName))
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
			kss.ksl.Debugf("getLeafCellbyNode: %+v", current)
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
