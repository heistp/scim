// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"time"
)

//
// Common Settings
//

// Sender: test duration
const Duration = 20 * time.Second

// IFace: rate and rate schedule
var Rate = 100 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(20 * time.Second), 10 * Mbps},
}

// Iface: AQM config
var UseAQM = NewDelmin(Clock(5000*time.Microsecond),
	Clock(10*time.Microsecond))

// var UseAQM = NewRamp()
var SCERampMin = Clock(TransferTime(Rate, Bytes(MSS))) * 1

const SCERampMax = Clock(100 * time.Millisecond)

// Sender: flows
var Flows = []Flow{
	AddFlow(NoSCE, true),
	AddFlow(SCE, true),
}
var FlowSchedule = []FlowAt{
	//FlowAt{1, Clock(10 * time.Second), true},
}

// Delay: path delays for each flow
const DefaultRTT = 20 * time.Millisecond

var FlowDelay = []Clock{
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
	Clock(DefaultRTT),
}

//
// Plots
//

// Sender: plots
const (
	PlotInFlight = false
	PlotCwnd     = true
	PlotRTT      = false
)

// Iface: plots
const (
	PlotSojourn     = true
	PlotQueueLength = false
)

// Delmin: plots
const (
	PlotDelminMarks = true
)

// Receiver: plots
const (
	PlotGoodput = true
)

//
// Less Common Settings
//

// Sender: SCE-MD params
const (
	CE_MD         = 0.5
	SCE_MD_Factor = 64
)

// Sender: TCP params
const (
	MSS      = 1500
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

// main
const Profile = false
