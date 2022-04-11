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
)

type Cell struct {
	cellType           string
	cellID             string
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
