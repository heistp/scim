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
		AddFlow(ECN, SCE, NewCUBIC(CMD), Pacing, NoHyStart, true),
	}
	FlowSchedule = []FlowAt{
		//FlowAt{1, Clock(10 * time.Second), true},
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
	}
)

// Sender: default responses
var (
	// CUBIC-SCE response
	CMD = MD(CubicBetaSCE)
	CRF = RateFairMD{CubicBeta, Clock(10 * time.Millisecond)}
	CHF = HybridFairMD{CubicBeta, Clock(10 * time.Millisecond)}

	// Reno-SCE Response
	RMD = MD(SCE_MD)
	RRF = RateFairMD{CEMD, Clock(10 * time.Millisecond)}
	RHF = HybridFairMD{CEMD, Clock(10 * time.Millisecond)}
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

// Iface: DelTiC AQM config
//var UseAQM = NewDeltic(Clock(5000 * time.Microsecond))

// Iface: DelTiM AQM config
var UseAQM = NewDeltim(Clock(5000 * time.Microsecond))

//var UseAQM = NewDeltim2(Clock(5000*time.Microsecond),
//	Clock(10*time.Microsecond))

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

// DelTiC: plots
const (
	PlotDelticMarks = true
	EmitDelticMarks = false
)

// DelTiM: plots
const (
	PlotDeltimMarks = true
	EmitDeltimMarks = false
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
	ScaleGrowth  = false // if true, use scalable cwnd growth
)

// Sender: Slow-Start params
//
// SlowStartExitMD: the MD done on slow-start exit, or 0 to calculate it from
// the cwnd and bytes ACKed.
//
// SlowStartExitCwndTargetingSCE*: on slow-start exit, target cwnd to the
// estimated available BDP by finding the in-flight bytes one RTT ago, and
// scaling that by minRtt/srtt.
const (
	SlowStartGrowth                   = SSGrowthABC2
	SlowStartExponentialBaseReduction = true
	SlowStartExitThreshold            = Tau / 2 // e.g. 0, Tau, Tau/2 or Tau/4
	SlowStartExitCwndTargetingSCE     = true
	SlowStartExitCwndTargetingNonSCE  = false
)

// SSGrowth selects the growth strategy for slow-start.
type SSGrowth int

const (
	SSGrowthNoABC  = iota // grow by one MSS per ACK
	SSGrowthABC1_5        // use ABC with base of 1.5, grow by 1/2 acked bytes
	SSGrowthABC2          // use ABC with base of 2, grow by acked bytes
)

// Sender: TCP params
const (
	MSS      = Bytes(1500)
	IW       = 10 * MSS
	RTTAlpha = float64(0.1)
)

// Sender: CUBIC params
const (
	CubicBeta            = 0.7  // RFC 9438 Section 4.6
	CubicC               = 0.4  // RFC 9438 Section 5
	CubicFastConvergence = true // RFC 9438 Section 4.7
)

// Sender: pacing params
//
// ThrottleSCEResponse only responds to SCE every RTT/Tau, and only when pacing
// is enabled.
const (
	PacingSSRatio       = float64(100) // Linux default == 200
	PacingCARatio       = float64(100) // Linux default == 120
	PacingCSSRatio      = float64(100)
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
