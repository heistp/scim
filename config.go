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
const Duration = 60 * time.Second

// Sender and Delay: flows
var (
	Flows = []Flow{
		AddFlow(ECN, SCE, Pacing, NoHyStart, true),
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
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		Clock(20 * time.Millisecond),
		//Clock(100 * 120 * time.Microsecond),
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

//var UseAQM = NewDeltim2(Clock(5000 * time.Microsecond))

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
	PlotGoodputPerRTT = 1
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

// Sender: SCE MD-Scaling params
const (
	BaseMD       = 0.5 // CE and drop
	Tau          = 64  // SCE-MD scale factor
	RateFairness = false
	NominalRTT   = 5 * time.Millisecond
)

// Sender: Slow-Start params
//
// SlowStartExitCwndAdjustment: on slow-start exit, scale cwnd by minRTT/maxRTT,
// noting that delayed ACKs can affect the results as RTT samples can be
// spuriously high, although see Flow.updateRTT() for the logic that uses
// the smoothed RTT to calculate maximums if delayed ACKs are enabled.
const (
	SlowStartExitThreshold      = 0 // e.g. 0, Tau or Tau / 2
	SlowStartExitCwndAdjustment = true
	CwndIncrementDivisor        = false
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
	HyMinRTTThresh     = Clock(2 * time.Millisecond) // default 4ms
	HyMaxRTTThresh     = Clock(8 * time.Millisecond) // default 16ms
	HyMinRTTDivisor    = 16                          // default 8
	HyNRTTSample       = 4                           // default 8
	HyCSSGrowthDivisor = 4                           // default 4
	HyCSSRounds        = 3                           // default 5
	HyStartLNoPacing   = 8                           // default 8
)

// main
const Profile = false
