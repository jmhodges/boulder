package rpc

import (
	"testing"
	"time"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/jmhodges/clock"
)

func TestRespChanMap(t *testing.T) {
	clk := clock.NewFake()
	cleanUpWait := 5 * time.Second
	rm := newRespChanMap(clk, cleanUpWait)
	rm.add("foobar", make(chan []byte))
	if rm.pending["foobar"] == nil {
		t.Errorf("didn't have foobar's channel in pending")
	}
	it, found := rm.get("foobar")
	if it == nil || !found {
		t.Errorf("get inaccurate: got (%v, %t) for foobar", it, found)
	}

	rm.markAsTimedOut("foobar")
	t.Logf("rm %#v", rm.timedOut.items)
	if len(rm.timedOut.items) != 1 {
		t.Errorf("foobar not moved to timedOut")
	}
	if len(rm.pending) != 1 {
		t.Errorf("marked as timed out should keep the responses in pending")
	}

	rm.cleanTimeouts()
	var d time.Duration
	for i := 0; i < 2; i++ {
		clk.Add(d)
		if len(rm.timedOut.items) != 1 {
			t.Errorf("cleanTimeouts removed the item from timedOut too soon: duration waited: %s", d)
		}
		if len(rm.pending) != 1 {
			t.Errorf("cleanTimeouts removed the channel from pending too soon: %s", d)
		}
		d = time.Duration(cleanUpWait)
	}

	clk.Add(1 * time.Nanosecond)
	rm.cleanTimeouts()
	if len(rm.timedOut.items) != 0 {
		t.Errorf("cleanTimeouts left item in timedOut")
	}
	if len(rm.pending) != 0 {
		t.Errorf("cleanTimeouts left channel in pending")
	}

	rm.add("foobar", make(chan []byte))
	rm.add("baz", make(chan []byte))
	rm.add("quux", make(chan []byte))
	rm.markAsTimedOut("foobar")
	rm.markAsTimedOut("baz")
	clk.Add(2 * cleanUpWait)
	rm.markAsTimedOut("quux")
	rm.cleanTimeouts()
	if rm.timedOut.items[0].coorID != "quux" {
		t.Errorf("cleanTimeouts didn't leave quux in last place: %s", rm.timedOut.items[0].coorID)
	}
	if len(rm.timedOut.items) != 1 {
		t.Errorf("cleanTimeouts should have only quux left over")
	}

	rm.markAsTimedOut("nope") // should not flip out
}
