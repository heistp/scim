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
const Duration = 10 * time.Second

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
	AddFlow(SCE, true),
	//AddFlow(SCE, true),
}
var FlowSchedule = []FlowAt{
	//FlowAt{1, Clock(10 * time.Second), false},
}

// Delay: path delays for each flow
var FlowDelay = []Clock{
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
}

//
// Plots
//

// Iface: plots
const (
	PlotSojourn     = true
	PlotMarks       = false
	PlotQueueLength = true
)

// Sender: plots
const (
	PlotInFlight = false
	PlotCwnd     = true
	PlotRTT      = false
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
