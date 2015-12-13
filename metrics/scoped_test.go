// Copyright 2015 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package metrics

import (
	"testing"
	"time"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/golang/mock/gomock"
	"github.com/letsencrypt/boulder/metrics/mock_statsd"
)

func TestScopedStatsStatsd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	statter := mock_statsd.NewMockStatter(ctrl)
	stats := NewScopedFromStatsd("fake", statter)
	statter.EXPECT().Inc("fake.counter", 2, 1.0).Return(nil)
	stats.Inc("counter", 2)

	statter.EXPECT().Dec("fake.counter", 2, 1.0).Return(nil)
	stats.Dec("counter", 2)

	statter.EXPECT().Gauge("fake.gauge", 2, 1.0).Return(nil)
	stats.Gauge("gauge", 2)
	statter.EXPECT().GaugeDelta("fake.delta", 2, 1.0).Return(nil)
	stats.GaugeDelta("delta", 2)
	statter.EXPECT().Timing("fake.latency", 2, 1.0).Return(nil)
	stats.Timing("latency", 2)
	statter.EXPECT().TimingDuration("fake.latency", 2*time.Second, 1.0).Return(nil)
	stats.TimingDuration("latency", 2*time.Second)
	statter.EXPECT().Set("fake.something", "value", 1.0).Return(nil)
	stats.Set("something", "value")
	statter.EXPECT().SetInt("fake.someint", 10, 1.0).Return(nil)
	stats.SetInt("someint", 10)
	statter.EXPECT().Raw("fake.raw", "raw value", 1.0).Return(nil)
	stats.Raw("raw", "raw value")

	s := stats.NewScope("foobar")
	statter.EXPECT().Inc("fake.foobar.counter", 3, 1.0).Return(nil)
	s.Inc("counter", 3)

	if stats.Scope() != "fake" {
		t.Errorf(`expected "fake", got %#v`, stats.Scope())
	}
	if s.Scope() != "fake.foobar" {
		t.Errorf(`expected "fake.foobar", got %#v`, s.Scope())
	}

}
