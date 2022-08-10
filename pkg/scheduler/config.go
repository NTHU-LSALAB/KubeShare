package scheduler

import (
	"os"
	"reflect"
	"strconv"

	"KubeShare/pkg/lib/queue"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

type Config struct {
	// specify the whole physical cluster
	// key: celltype -> val: cellTypeSpec
	// TODO: Automatically construct it based on node info from Device Plugins
	CellTypes map[string]CellTypeSpec `yaml:"cellTypes"`
	Cells     []CellSpec              `yaml:"cells"`
}

type CellTypeSpec struct {
	ChildCellType     string `yaml:"childCellType"`
	ChildCellNumber   int32  `yaml:"childCellNumber"`
	ChildCellPriority int32  `yaml:"childCellPriority"`
	IsNodeLevel       bool   `yaml:"isNodeLevel"`
}

// Specify Cell instances -> node-level or above
type CellSpec struct {
	CellType     string     `yaml:"cellType"`
	CellID       string     `yaml:"cellId"`
	CellChildren []CellSpec `yaml:"cellChildren,omitempty"`
}

func (kss *KubeShareScheduler) initRawConfig() *Config {
	var c Config

	configPath := kss.args.kubeShareConfig
	// convert raw data to yaml
	yamlBytes, err := os.ReadFile(configPath)
	if err != nil {
		kss.ksl.Errorf("Failed to read config file: %v, %v", configPath, err)
	}

	if err := yaml.Unmarshal(yamlBytes, &c); err != nil {
		kss.ksl.Errorf("Failed to unmarshal YAML %#v to object: %v", string(yamlBytes), err)
	}

	if c.CellTypes == nil || c.Cells == nil {
		kss.ksl.Warn("The cellTypes and cells in kubeshare config file is nil")
	}

	kss.checkPhysicalCells(&c)
	return &c
}

func (kss *KubeShareScheduler) checkPhysicalCells(c *Config) {
	// check if cell type if physical cells map into celltypes
	cellTypes := c.CellTypes
	cells := c.Cells

	for idx, cell := range cells {
		cts, ok := cellTypes[cell.CellType]
		if !ok {
			kss.ksl.Errorf("Cells contains unknown cellType: %v", cell.CellType)
		}
		if cts.ChildCellPriority > 100 || cts.ChildCellPriority < 0 {
			kss.ksl.Errorf("Cell Priority must be in  0~1000")
		}
		inferCellSpec(&cells[idx], cellTypes, cell.CellType, idx+1)
	}
}

// infer the child's configuration from the parent's configuration
func inferCellSpec(spec *CellSpec, cellTypes map[string]CellTypeSpec, cellType string, defaultID int) {
	idQueue := queue.NewQueue()
	q := queue.NewQueue()
	q.Enqueue(spec)
	first := true

	for q.Len() > 0 {
		n := q.Len()
		for i := 1; i <= n; i++ {
			current := q.Dequeue().(*CellSpec)
			if first {
				if current.CellID == "" {
					current.CellID = current.CellID + strconv.Itoa(defaultID)
				}
				first = false
			} else {
				previousID := idQueue.Dequeue().(string)
				if current.CellID == "" {
					current.CellID = previousID + "/" + strconv.Itoa(int(i))
				} else {
					current.CellID = previousID + "/" + current.CellID
				}
			}

			// if we can't find the cellType in celltypes,
			// it means current celltpe is a leaf cell type.
			ct, ok := cellTypes[current.CellType]
			if !ok {
				continue
			}
			if ct.ChildCellNumber > 0 && len(current.CellChildren) == 0 {
				current.CellChildren = make([]CellSpec, ct.ChildCellNumber)
			}
			for c := range current.CellChildren {
				if current.CellChildren[c].CellType == "" {
					current.CellChildren[c].CellType = ct.ChildCellType
				}

				idQueue.Enqueue(current.CellID)
				q.Enqueue(&current.CellChildren[c])
			}
		}
	}
}

func (kss *KubeShareScheduler) watchConfig(c *Config) {
	configPath := kss.args.kubeShareConfig
	v := viper.New()
	v.SetConfigFile(configPath)
	v.WatchConfig()
	kss.ksl.Info("Watching config file: ", configPath)

	v.OnConfigChange(func(e fsnotify.Event) {
		kss.ksl.Warnf("Watched config file is changed: ", e.Name)
		if ok := reflect.DeepEqual(*c, kss.initRawConfig()); !ok {
			kss.ksl.Error("Config file content is changed, exiting ...")
			os.Exit(0)
		}
	})
}
