// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"time"
)

// Sender
const (
	Duration      = 30 * time.Second
	CE_MD         = 0.5
	SCE_MD_Factor = 64

	PlotInFlight = false
	PlotCwnd     = true
	PlotRTT      = false

	MSS      = 1500
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

var Flows = []Flow{
	AddFlow(true),
	AddFlow(true),
}

// Iface
const (
	Rate = 100 * Mbps

	PlotSojourn = true
	PlotMarks   = false
)

// Delmin
// var UseAQM = NewDelmin1(Clock(5 * time.Millisecond))
var UseAQM = NewDelmin2(Clock(5*time.Millisecond),
	Clock(100*time.Microsecond))

// Ramp
// var UseAQM = NewRamp()
var SCERampMin = Clock(TransferTime(Rate, Bytes(MSS))) * 1

const SCERampMax = Clock(100 * time.Millisecond)

// Delay
var FlowDelay = []Clock{
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
}

// Receiver
const (
	PlotGoodput = true
)

// main
const Profile = false
