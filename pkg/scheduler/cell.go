package scheduler

type (
	CellState string
	CellList  []Cell
	// maps each level to cellList
	LevelCellList map[int]CellList
)

const (
	CellFree     CellState = "FREE"
	CellReserved CellState = "RESERVED"
	lowestLevel  int       = 1
)

type Cell struct {
	cellType           string
	id                 string
	level              int
	atOrHigherThanNode bool // true if the cell is at or higher than node level

	capacity   float64
	allocation float64

	priority int32
	uuid     string
	model    string

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
	capacity float64,
	priority int32,
	model string,

) *Cell {
	return &Cell{
		cellType:           cellType,
		id:                 id,
		level:              level,
		atOrHigherThanNode: atOrHigherThanNode,
		capacity:           capacity,
		allocation:         0.0,
		priority:           priority,
		uuid:               "",
		model:              model,
		healthy:            false,
		state:              CellFree,
	}
}

// internal structure to build the cell tree
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
	return cellElements
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
		return
	}

	// recursively add children
	child := cts.ChildCellType
	priority = cts.ChildCellPriority

	kss.gpuPriorityMutex.Lock()
	defer kss.gpuPriorityMutex.Unlock()
	kss.gpuPriority[child] = priority
	kss.ksl.Debugf("gpu priority %v = %v", child, priority)

	if _, ok := cellElements[child]; !ok {
		kss.addCell(child, cellTypes, cellElements, priority)
	}

	// child cell type has been added, add current element
	childCellElement := cellElements[child]
	cellElements[cellType] = &cellElement{
		cellType:           cellType,
		level:              childCellElement.level + 1,
		priority:           childCellElement.priority,
		childCellType:      childCellElement.childCellType,
		childCellNumber:    float64(cts.ChildCellNumber),
		leafCellType:       childCellElement.leafCellType,
		leafCellNumber:     childCellElement.leafCellNumber * float64(cts.ChildCellNumber),
		atOrHigherThanNode: childCellElement.atOrHigherThanNode || cts.IsNodeLevel,
		isMultiNodes:       childCellElement.atOrHigherThanNode,
	}

}
