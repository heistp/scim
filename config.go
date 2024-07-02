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
		//FlowAt{0, Clock(30 * time.Second), true},
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
//var UseAQM = NewDeltim(Clock(5000*time.Microsecond),
//	Clock(10*time.Microsecond))

var UseAQM = NewDeltim2(Clock(5000 * time.Microsecond))

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
	PlotInFlight     = false
	PlotCwnd         = true
	PlotCwndInterval = Clock(100 * time.Microsecond)
	PlotRTT          = false
)

// Iface: plots
const (
	PlotSojourn         = true
	PlotSojournInterval = Clock(100 * time.Microsecond)
	PlotQueueLength     = false
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
	DropMD       = 0.5    // MD done on drop during CA
	CEMD         = DropMD // MD done on CE during CA
	Tau          = 64     // SCE-MD scale factor
	RateFairness = false
	NominalRTT   = 10 * time.Millisecond
	ScaleGrowth  = true // if true, use scalable cwnd growth
)

// Sender: Slow-Start params
//
// SlowStartExitMD: the MD done on slow-start exit, or 0 to calculate it from
// the cwnd and bytes ACKed.
//
// SlowStartExitCwndAdjustment*: on slow-start exit, scale cwnd by
// minRTT/maxRTT.  For accuracy, it's important to have pacing enabled.  Also
// note that delayed ACKs can affect the results as RTT samples can be
// spuriously high, although see Flow.updateRTT() for the logic that uses the
// smoothed RTT to calculate maximums if delayed ACKs are enabled.
const (
	SlowStartGrowth                   = SSGrowthABC2
	SlowStartCwndIncrementDivisor     = false
	SlowStartExitThreshold            = Tau / 2 // e.g. 0, Tau, Tau/2 or Tau/4
	SlowStartExitMD                   = float64(0)
	SlowStartExitCwndAdjustmentSCE    = true
	SlowStartExitCwndAdjustmentNonSCE = false
)

// SSGrowth selects the growth strategy for slow-start.
type SSGrowth int

const (
	SSGrowthNoABC = iota // grow by one MSS per ACK
	SSGrowthABC15        // use ABC with base of 1.5, grow by 1/2 acked bytes
	SSGrowthABC2         // use ABC with base of 2, grow by acked bytes
)

// Sender: TCP params
const (
	MSS      = Bytes(1500)
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

// Sender: pacing params
//
// ThrottleSCEResponse only responds to SCE every RTT/Tau, and only when pacing
// is enabled.
const (
	PacingSSRatio       = float64(100)
	PacingCSSRatio      = float64(100)
	PacingCARatio       = float64(100)
	ThrottleSCEResponse = false
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
