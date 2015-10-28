package rpc

import (
	"container/heap"
	"sync"
	"time"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/jmhodges/clock"
)

type respChanMap struct {
	sync.RWMutex
	clk      clock.Clock
	pending  map[string]chan []byte
	timedOut *timedOutQueue
}

func newRespChanMap(clk clock.Clock, cleanUpWait time.Duration) *respChanMap {
	q := &timedOutQueue{clk: clk, cleanUpWait: cleanUpWait}
	heap.Init(q)
	return &respChanMap{
		pending:  make(map[string]chan []byte),
		clk:      clk,
		timedOut: q,
	}
}

func spawnTimeoutCleaner(tickInterval time.Duration, rm *respChanMap) {
	ticker := time.NewTicker(tickInterval)
	for _ = range ticker.C {
		rm.cleanTimeouts()
	}
}

func (rm *respChanMap) add(corrID string, responseChan chan []byte) {
	rm.Lock()
	defer rm.Unlock()
	rm.pending[corrID] = responseChan
}

func (rm *respChanMap) get(coorID string) (chan []byte, bool) {
	rm.RLock()
	defer rm.RUnlock()
	ch, found := rm.pending[coorID]
	return ch, found
}

func (rm *respChanMap) del(corrID string) {
	rm.Lock()
	defer rm.Unlock()
	delete(rm.pending, corrID)
}

func (rm *respChanMap) markAsTimedOut(coorID string) {
	rm.Lock()
	defer rm.Unlock()
	_, found := rm.pending[coorID]
	if !found {
		return
	}
	heap.Push(rm.timedOut, &timedOutItem{rm.clk.Now(), coorID})
}

func (rm *respChanMap) cleanTimeouts() {
	for {
		rm.RLock()
		nothingToDo := rm.timedOut.Len() == 0
		rm.RUnlock()
		if nothingToDo {
			return
		}
		rm.Lock()
		x := heap.Pop(rm.timedOut)
		rm.Unlock()
		if x == nil {
			break
		}
		item := x.(*timedOutItem)
		rm.del(item.coorID)
	}
}

type timedOutItem struct {
	t      time.Time
	coorID string
}

// timedOutQueue is a heap of request ids ordered by the time that they were
// timed out. Non-nil items returned by Pop from the queue are items with
// request ids whose response channels can be cleared from the pending field in
// respChanMap.
type timedOutQueue struct {
	clk clock.Clock
	// cleanUpWait is how long after a timeout to wait before cleaing up the
	// pending map entry
	cleanUpWait time.Duration
	items       []*timedOutItem
}

func (tq timedOutQueue) Len() int { return len(tq.items) }

func (tq timedOutQueue) Less(i, j int) bool {
	// We want the lowest timeout to be returned by Pop, so we use less than here
	return tq.items[i].t.Before(tq.items[j].t)
}

func (tq timedOutQueue) Swap(i, j int) {
	tq.items[i], tq.items[j] = tq.items[j], tq.items[i]
}

func (tq *timedOutQueue) Push(x interface{}) {
	item := x.(*timedOutItem)
	tq.items = append(tq.items, item)
}

// Pop returns the first timedOutItem that has timed out more than
// ago. It will return nil if none match that criteria.
func (tq *timedOutQueue) Pop() interface{} {
	n := len(tq.items)
	item := tq.items[n-1]
	if item.t.Before(tq.clk.Now().Add(-tq.cleanUpWait)) {
		tq.items = tq.items[0 : n-1]
		return item
	}
	return nil
}
