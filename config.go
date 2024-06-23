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
		AddFlow(SCE, NoPacing, HyStart, true),
	}
	FlowSchedule = []FlowAt{
		//FlowAt{0, Clock(20 * time.Second), false},
	}
	FlowDelay = []Clock{
		Clock(20 * time.Millisecond),
		//Clock(100 * 120 * time.Microsecond),
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
var RateInit = 100 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 100 * Mbps},
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

// Receiver: delayed ACKs (>0 enables delayed ACKs)
//
// QuickACKSignal: if true, always ACK SCE or CE immediately, otherwise use
// "Advanced" ACK handling and quick ACK changes in SCE or CE state.  Only
// applies when delayed ACKs are enabled.
const (
	//DelayedACKTime = Clock(40 * time.Millisecond)
	DelayedACKTime = Clock(0)
	QuickACKSignal = true
)

//
// Less Common Settings
//

// Sender: SCE-AIMD params
const (
	CE_MD                  = 0.8
	Tau                    = 64 // SCE-MD scale factor
	RateFairness           = false
	NominalRTT             = 20 * time.Millisecond
	SlowStartExitThreshold = 0 // e.g. Tau or Tau / 2
)

// Sender: TCP params
const (
	MSS      = Bytes(1500)
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

// Sender: pacing params
const (
	PacingSSRatio  = float64(100)
	PacingCSSRatio = float64(100)
	PacingCARatio  = float64(100)
)

// Sender: HyStart++ (RFC 9406)
const (
	HyStartPP          = true
	HyMinRTTThresh     = Clock(4 * time.Millisecond)
	HyMaxRTTThresh     = Clock(16 * time.Millisecond)
	HyMinRTTDivisor    = 8
	HyNRTTSample       = 8
	HyCSSGrowthDivisor = 4
	HyCSSRounds        = 5
	HyStartLNoPacing   = 8
)

// main
const Profile = false
