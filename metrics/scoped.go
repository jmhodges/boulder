// Copyright 2015 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate sh -c "mockgen github.com/cactus/go-statsd-client/statsd Statter > ./mock_statsd/mock_statsd.go && sed -i '' -e 's:github.com/golang/mock/gomock:github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/golang/mock/gomock:' ./mock_statsd/mock_statsd.go"

package metrics

import (
	"strings"
	"time"

	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/cactus/go-statsd-client/statsd"
)

type Scoped interface {
	NewScope(scopes ...string) Scoped
	Scope() string

	Inc(stat string, value int64) error
	Dec(stat string, value int64) error
	Gauge(stat string, value int64) error
	GaugeDelta(stat string, value int64) error
	Timing(stat string, delta int64) error
	TimingDuration(stat string, delta time.Duration) error
	Set(stat string, value string) error
	SetInt(stat string, value int64) error
	Raw(stat string, value string) error
}

type ScopedStatsd struct {
	prefix  string
	statter statsd.Statter
}

var _ Scoped = &ScopedStatsd{}

func NewScopedFromStatsd(statter statsd.Statter, scopes ...string) *ScopedStatsd {
	return &ScopedStatsd{
		prefix:  strings.Join(scopes, ".") + ".",
		statter: statter,
	}
}
func NewNoopScoped() Scoped {
	c, _ := statsd.NewNoopClient()
	return NewScopedFromStatsd("noop", c)
}

func (s *ScopedStatsd) NewScope(scopes ...string) Scoped {
	scope := strings.Join(scopes, ".")
	return NewScopedFromStatsd(s.prefix+scope, s.statter)
}

func (s *ScopedStatsd) Scope() string {
	return s.prefix[:len(s.prefix)-1]
}

func (s *ScopedStatsd) Inc(stat string, value int64) error {
	return s.statter.Inc(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) Dec(stat string, value int64) error {
	return s.statter.Dec(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) Gauge(stat string, value int64) error {
	return s.statter.Gauge(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) GaugeDelta(stat string, value int64) error {
	return s.statter.GaugeDelta(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) Timing(stat string, delta int64) error {
	return s.statter.Timing(s.prefix+stat, delta, 1.0)
}
func (s *ScopedStatsd) TimingDuration(stat string, delta time.Duration) error {
	return s.statter.TimingDuration(s.prefix+stat, delta, 1.0)
}
func (s *ScopedStatsd) Set(stat string, value string) error {
	return s.statter.Set(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) SetInt(stat string, value int64) error {
	return s.statter.SetInt(s.prefix+stat, value, 1.0)
}
func (s *ScopedStatsd) Raw(stat string, value string) error {
	return s.statter.Raw(s.prefix+stat, value, 1.0)
}
