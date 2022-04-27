package scheduler

import (
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

type (
	CellState string
	CellList  []*Cell
	// maps each level to cellList
	LevelCellList map[int]CellList
)

const (
	CellFree     CellState = "FREE"
	CellReserved CellState = "RESERVED"
	CellPartial  CellState = "PARTIAL"
	CellUsed     CellState = "USED"
	lowestLevel  int       = 1
)

// internal structure to build the cell elements
// preprocess the information about the cell
type cellElement struct {
	cellType           string  // cell types
	level              int     // cell level, leaf cell is 1
	priority           int32   // cell priority
	childCellNumber    float64 // how many child cells in the current cell
	childCellType      string  // child cell type
	leafCellNumber     float64 // how many leaf cells in the current cell
	leafCellType       string  // leaf cell type
	atOrHigherThanNode bool    // cell types is a node or above cell
	isMultiNodes       bool    // cell type is a multiple cell
}

func (kss *KubeShareScheduler) buildCellChains(cellTypes map[string]CellTypeSpec) map[string]*cellElement {
	cellElements := map[string]*cellElement{}
	for cellType := range cellTypes {
		kss.addCell(cellType, cellTypes, cellElements, 1)
	}

	kss.sortGPUPriority()

	return cellElements
}

func (kss *KubeShareScheduler) sortGPUPriority() {
	models := make([]string, 0, len(kss.gpuPriority))
	prioriries := kss.gpuPriority
	for model := range prioriries {
		models = append(models, model)
	}

	sort.SliceStable(models, func(i, j int) bool {
		return prioriries[models[i]] > prioriries[models[j]]
	})
	kss.sortGPUByPriority = models

	for _, model := range kss.sortGPUByPriority {
		kss.ksl.Infof("GPU priority: %v\t%v", model, kss.gpuPriority[model])
	}
}

func (kss *KubeShareScheduler) addCell(
	cellType string,
	cellTypes map[string]CellTypeSpec,
	cellElements map[string]*cellElement,
	priority int32) {
	kss.ksl.Debugf("[addCell] %v", cellType)
	// check whether the celltype has added in cellElements
	_, ok := cellElements[cellType]
	// already added
	if ok {
		return
	}
	cts, ok := cellTypes[cellType]
	// not found in raw spec, it's leaf cell
	if !ok {
		cellElements[cellType] = &cellElement{
			cellType:           cellType,
			level:              lowestLevel,
			priority:           priority,
			childCellType:      "",
			childCellNumber:    0.0,
			leafCellType:       cellType,
			leafCellNumber:     1.0,
			atOrHigherThanNode: false,
			isMultiNodes:       false,
		}

		// kss.gpuPriorityMutex.Lock()
		// defer kss.gpuPriorityMutex.Unlock()
		kss.gpuPriority[cellType] = priority
		return
	}

	// recursively add children
	child := cts.ChildCellType
	priority = cts.ChildCellPriority

	if _, ok := cellElements[child]; !ok {
		kss.addCell(child, cellTypes, cellElements, priority)
	}

	// child cell type has been added, add current element
	childCellElement := cellElements[child]
	cellElements[cellType] = &cellElement{
		cellType:           cellType,
		level:              childCellElement.level + 1,
		priority:           childCellElement.priority,
		childCellType:      childCellElement.cellType,
		childCellNumber:    float64(cts.ChildCellNumber),
		leafCellType:       childCellElement.leafCellType,
		leafCellNumber:     childCellElement.leafCellNumber * float64(cts.ChildCellNumber),
		atOrHigherThanNode: childCellElement.atOrHigherThanNode || cts.IsNodeLevel,
		isMultiNodes:       childCellElement.atOrHigherThanNode,
	}

}

type Cell struct {
	cellType           string
	id                 string
	level              int
	atOrHigherThanNode bool // true if the cell is at or higher than node level

	priority int32
	uuid     string

	leafCellType       string
	leafCellNumber     float64
	availableWholeCell float64 // a number of whole cells are available
	freeMemory         int64
	fullMemory         int64
	available          float64 // remaining gpu resource
	node               string

	healthy bool
	state   CellState

	parent *Cell    // pointer to its parent cell
	child  CellList // pointer to its children cells
}

func NewCell(
	cellType string,
	id string,
	level int,
	atOrHigherThanNode bool,
	leafCellNumber float64,
	priority int32,
	leafCellType string,

) *Cell {
	return &Cell{
		cellType:           cellType,
		id:                 id,
		level:              level,
		atOrHigherThanNode: atOrHigherThanNode,
		priority:           priority,
		uuid:               "",
		freeMemory:         0,
		fullMemory:         0,
		available:          leafCellNumber,
		availableWholeCell: leafCellNumber,
		leafCellType:       leafCellType,
		leafCellNumber:     leafCellNumber,
		healthy:            false,
		state:              CellFree,
	}
}

func NewCellList(top int) LevelCellList {
	ccl := LevelCellList{}
	for i := lowestLevel; i <= top; i++ {
		ccl[i] = CellList{}
	}
	return ccl
}

type cellConstructor struct {
	// input
	cellElements map[string]*cellElement
	cells        []CellSpec

	// output
	cellFreeList map[string]LevelCellList

	// logger
	ksl *logrus.Logger
}

func newCellConstructor(cellElements map[string]*cellElement, cells []CellSpec, ksl *logrus.Logger) *cellConstructor {
	return &cellConstructor{
		cellElements: cellElements,
		cells:        cells,
		cellFreeList: map[string]LevelCellList{},
		ksl:          ksl,
	}
}

func (c *cellConstructor) build() (cellFreeList map[string]LevelCellList) {
	for _, spec := range c.cells {

		rootCell := c.buildFullTree(spec.CellType, spec)
		cellType := rootCell.leafCellType
		cellLevel := rootCell.level

		// initialize the cell free list
		if _, ok := c.cellFreeList[cellType]; !ok {
			c.cellFreeList[cellType] = NewCellList(cellLevel)
		}
		c.cellFreeList[cellType][cellLevel] = append(
			c.cellFreeList[cellType][cellLevel], rootCell)
	}
	return c.cellFreeList
}

func (c *cellConstructor) buildFullTree(buildingType string, buildingSpec CellSpec) *Cell {

	//check the cellElement of the cellType
	ce, ok := c.cellElements[buildingType]
	if !ok {
		c.ksl.Errorf("cellType %v in Cells is not found in cell types definition", buildingType)
	}
	// make sure cells in kubeshare-config.yaml will start from node or above
	if !ce.atOrHigherThanNode {
		c.ksl.Errorf("top cell must be node-level or above: %v", buildingType)
	}

	cellInstance := c.buildChildCell(buildingSpec, buildingType, "")
	cellInstance.leafCellType = ce.leafCellType

	return cellInstance
}

func (c *cellConstructor) buildChildCell(
	spec CellSpec,
	cellType string,
	currentNode string) *Cell {

	ce := c.cellElements[cellType]
	// node-level: pass node name it to its child
	if ce.atOrHigherThanNode && !ce.isMultiNodes {
		splitID := strings.Split(string(spec.CellID), "/")
		currentNode = splitID[len(splitID)-1]
	}
	cellInstance := NewCell(cellType, spec.CellID, ce.level, ce.atOrHigherThanNode || ce.isMultiNodes, ce.leafCellNumber, ce.priority, ce.leafCellType)
	if !ce.isMultiNodes {
		cellInstance.node = currentNode
	}

	if ce.level == 1 {
		c.ksl.Debugf("%+v", cellInstance)
		return cellInstance
	}

	var currentCellChildren CellList
	for _, childSpec := range spec.CellChildren {
		childCellInstance := c.buildChildCell(childSpec, ce.childCellType, currentNode)
		childCellInstance.parent = cellInstance
		if !ce.isMultiNodes {
			childCellInstance.node = currentNode
		}
		currentCellChildren = append(currentCellChildren, childCellInstance)
		// c.ksl.Debugf("%+v", childCellInstance)
	}

	// update current cell children and resource
	cellInstance.child = currentCellChildren
	c.ksl.Debugf("%+v", cellInstance)

	return cellInstance
}
