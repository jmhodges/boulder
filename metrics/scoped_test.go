// Copyright 2015 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package metrics

import (
	"testing"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/golang/mock/gomock"
	"github.com/letsencrypt/boulder/metrics/mock_statsd"
)

func TestScopedStatsStatsd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	statter := mock_statsd.NewMockStatter(ctrl)
	stats := NewStatsFromStatsd("fake", statter)
	statter.EXPECT().Inc("fake.counter", 2, 1.0).Return(nil)
	stats.Inc("counter", 2)

	statter.EXPECT().Dec("fake.counter", 2, 1.0).Return(nil)
	stats.Dec("counter", 2)
}
