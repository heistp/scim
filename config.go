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
	}
	FlowSchedule = []FlowAt{
		//FlowAt{0, Clock(20 * time.Second), false},
	}
	FlowDelay = []Clock{
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
	}
)

// IFace: initial rate and rate schedule
var RateInit = 50 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 50 * Mbps},
}

//func init() {
//	for t := 5 * Clock(time.Second); t < 100*Clock(time.Second); t += 5 * Clock(time.Second) {
//		RateSchedule = append(RateSchedule, RateAt{t,
//			Bitrate(2*time.Duration(t).Seconds()) * Mbps})
//	}
//}

// Iface: DelTiM AQM config
var UseAQM = NewDeltim(Clock(5000*time.Microsecond),
	Clock(10*time.Microsecond))

// Iface: Ramp AQM config
var (
	//UseAQM     = NewRamp()
	SCERampMin = Clock(TransferTime(RateInit, Bytes(MSS))) * 1
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

// DelTiM: plots
const (
	PlotDeltimMarks = true
	EmitMarks       = false
)

// Receiver: plots
const (
	PlotGoodput       = true
	PlotGoodputPerRTT = 2
)

//
// Less Common Settings
//

// Sender: SCE-MD params
const (
	CE_MD        = 0.8
	SCE_MD_Scale = 64
	NominalRTT   = 20 * time.Millisecond
	RateFairness = false
)

// Sender: TCP params
const (
	MSS      = 1500
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

// main
const Profile = false
