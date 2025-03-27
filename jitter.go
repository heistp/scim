// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import "time"

// jitterEstimator estimates jitter
type jitterEstimator struct {
	jitter Clock
	prior  Clock
}

// estimate returns the estimated jitter.
func (j *jitterEstimator) estimate(now Clock) Clock {
	i := min(max(0, now-j.prior), Clock(time.Second))
	ab := i*i + j.jitter*(Clock(time.Second)-i)
	j.jitter = ab / Clock(time.Second)
	j.prior = now
	return j.jitter
}

// adjustSojourn returns the given sojourn time with any jitter "forgiven".
func (j *jitterEstimator) adjustSojourn(sojourn Clock) Clock {
	if sojourn <= j.jitter {
		return 0
	}
	return sojourn - j.jitter
}
