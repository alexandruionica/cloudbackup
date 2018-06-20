package globals

import (
	"sync"

	"cloudbackup/utils"
	log "github.com/sirupsen/logrus"
)

const loggingContext = "daemon"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})
var Stats GlobalStats

type Gauge struct{
	Routines map[string]uint64 `json:"routines"`
	Functions map[string]uint64 `json:"functions"`
}

type Counter struct{
	Routines map[string]uint64 `json:"routines"`
	Functions map[string]uint64 `json:"functions"`
}
type GlobalStats struct {
	Gauge Gauge `json:"gauge"`
	Counter Counter `json:"counter"`
	Lock *sync.RWMutex `json:"-"`
}

func (stats *GlobalStats) IncrementRoutines(name string) {
	stats.Lock.Lock()
	defer func() {
		stats.Lock.Unlock()
	}()
	stats.Gauge.Routines[name] += 1
	stats.Counter.Routines[name] += 1
}

func (stats *GlobalStats) DecrementRoutines(name string) {
	stats.Lock.Lock()
	defer func() {
		stats.Lock.Unlock()
	}()
	if stats.Gauge.Routines[name] == 0 {
		logger.Warnf("GlobalStats.Gauge.Routines.%s already 0 but attempted to decrement", name)
		return
	}
	stats.Gauge.Routines[name] -= 1
}

func (stats *GlobalStats) IncrementFunctions(name string) {
	stats.Lock.Lock()
	defer func() {
		stats.Lock.Unlock()
	}()
	stats.Gauge.Functions[name] += 1
	stats.Counter.Functions[name] += 1
}

func (stats *GlobalStats) DecrementFunctions(name string) {
	stats.Lock.Lock()
	defer func() {
		stats.Lock.Unlock()
	}()
	if stats.Gauge.Functions[name] == 0 {
		logger.Warnf("GlobalStats.Functions.Routines.%s already 0 but attempted to decrement", name)
		return
	}
	stats.Gauge.Functions[name] -= 1
}

// pretty prints to stdout the whole contents of the stats
func (stats *GlobalStats) Print() {
	stats.Lock.RLock()
	defer func() {
		stats.Lock.RUnlock()
	}()
	utils.Pp(stats)
}


// logs stats and also print to stdout
func (stats *GlobalStats) Log() {
	stats.Lock.RLock()
	defer func() {
		stats.Lock.RUnlock()
	}()
	logger.Infof("%+v", stats)
	utils.Pp(stats)
}

// when the package is loaded, automatically setup a stats object; further loads of the package don't trigger new loads
func init() {
	Stats = GlobalStats{
		Gauge: Gauge{
			Routines: map[string]uint64{},
			Functions: map[string]uint64{},
		},
		Counter: Counter{
			Routines: map[string]uint64{},
			Functions: map[string]uint64{},
		},
		Lock: &sync.RWMutex{},
	}
}