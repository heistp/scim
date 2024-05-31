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
const Duration = 30 * time.Second

// Sender and Delay: flows
var (
	Flows = []Flow{
		AddFlow(SCE, true),
		AddFlow(NoSCE, true),
	}
	FlowSchedule = []FlowAt{
		//FlowAt{1, Clock(10 * time.Second), true},
	}
	FlowDelay = []Clock{
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
	}
)

// IFace: rate and rate schedule
var Rate = 100 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 10 * Mbps},
}

// Iface: Delmin AQM config
var UseAQM = NewDelmin(Clock(5000*time.Microsecond),
	Clock(10*time.Microsecond))

// Iface: Ramp AQM config
var (
	//UseAQM     = NewRamp()
	SCERampMin = Clock(TransferTime(Rate, Bytes(MSS))) * 1
	SCERampMax = Clock(100 * time.Millisecond)
)

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
	EmitMarks       = false
)

// Receiver: plots
const (
	PlotGoodput       = true
	PlotGoodputPerRTT = 8
)

//
// Less Common Settings
//

// Sender: SCE-MD params
const (
	CE_MD         = 0.8
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
