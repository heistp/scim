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
var Rate = 1 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 200 * Mbps},
}

// Iface: AQM config
// var UseAQM = NewDelmin1(Clock(5 * time.Millisecond))
var UseAQM = NewDelmin2(Clock(25000*time.Microsecond),
	Clock(500*time.Microsecond))

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
	PlotSojourn = true
	PlotMarks   = false
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
