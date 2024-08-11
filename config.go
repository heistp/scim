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
		AddFlow(ECN, SCE, NewEssp(), NoResponse{}, NewMaslo(), Pacing, true),
		//AddFlow(ECN, SCE, NewStdSS(), TargetCWND{}, NewReno(RMD), Pacing, true),
		//AddFlow(ECN, SCE, NewStdSS(), TargetCWND{}, NewReno2(RMD), Pacing, true),
		//AddFlow(ECN, SCE, NewStdSS(), TargetCWND{}, NewCUBIC(CMD), Pacing, true),
		//AddFlow(ECN, SCE, NewStdSS(), TargetCWND{}, NewScalable(RMD, 200), Pacing, true),
	}
	FlowSchedule = []FlowAt{
		//FlowAt{1, Clock(30 * time.Second), true},
		//FlowAt{1, Clock(60 * time.Second), false},
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
	CMF = MildFairMD{CubicBeta, Clock(10 * time.Millisecond)}

	// Reno-SCE Response
	RMD = MD(SCE_MD)
	RRF = RateFairMD{CEMD, Clock(10 * time.Millisecond)}
	RHF = HybridFairMD{CEMD, Clock(10 * time.Millisecond)}
	RMF = MildFairMD{CEMD, Clock(10 * time.Millisecond)}
)

// IFace: initial rate and rate schedule
var RateInit = 100 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 10 * Mbps},
	//RateAt{Clock(40 * time.Second), 100 * Mbps},
}

//func init() {
//	for t := 5 * Clock(time.Second); t < 100*Clock(time.Second); t += 5 * Clock(time.Second) {
//		RateSchedule = append(RateSchedule, RateAt{t,
//			Bitrate(2*time.Duration(t).Seconds()) * Mbps})
//	}
//}

// Iface: DelTiC AQM config
var UseAQM = NewDeltic(
	Clock(5*time.Millisecond),   // SCE
	Clock(25*time.Millisecond),  // CE
	Clock(125*time.Millisecond), // drop
)

// Iface: DelTiC-MDS AQM config
//var UseAQM = NewDelticMDS(Clock(5000 * time.Microsecond))

// Iface: DelTiM AQM config
//var UseAQM = NewDeltim(Clock(5000 * time.Microsecond))

// Iface: DelTiM2 AQM config
//var UseAQM = NewDeltim2(Clock(5000*time.Microsecond),
//	Clock(10*time.Microsecond))

// Iface: DelTiM3 AQM config
var (
	//UseAQM           = NewDeltim3(Clock(5000 * time.Microsecond))
	DeltimIdleWindow = Clock(20 * time.Millisecond)
)

// Iface: Brickwall AQM config
//var UseAQM = NewBrickwall(
//	Clock(0*time.Millisecond),   // SCE
//	Clock(400*time.Millisecond), // CE
//	Clock(0*time.Millisecond),   // drop
//)

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
	PlotInFlight         = false
	PlotInFlightInterval = Clock(100 * time.Microsecond)
	PlotCwnd             = true
	PlotCwndInterval     = Clock(100 * time.Microsecond)
	PlotRTT              = false
	PlotRTTInterval      = Clock(100 * time.Microsecond)
	PlotPacing           = true
	PlotPacingInterval   = Clock(100 * time.Microsecond)
	PlotSeq              = false
	PlotSeqInterval      = Clock(100 * time.Microsecond)
	PlotSent             = false
	PlotSentInterval     = Clock(100 * time.Microsecond)
	PlotRate             = false
	PlotRateInterval     = Clock(100 * time.Microsecond)
)

// Iface: plots
const (
	PlotSojourn             = true
	PlotSojournInterval     = Clock(100 * time.Microsecond)
	PlotQueueLength         = false
	PlotQueueLengthInterval = Clock(100 * time.Microsecond)
	PlotDeltaSigma          = false
	PlotByteSeconds         = false
	PlotByteSecondsInterval = Clock(100 * time.Microsecond)
)

// AQM: plots
const (
	PlotMarkProportion = false
	PlotMarkFrequency  = true
	EmitMark           = false
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
	DropMD = 0.5    // MD done on drop during CA
	CEMD   = DropMD // MD done on CE during CA
	Tau    = 64     // SCE-MD scale factor
)

// Sender: Slow-Start defaults
const (
	DefaultSSGrowth        = SSGrowthABC2
	DefaultSSBaseReduction = true
	DefaultSSExitThreshold = Tau / 2 // e.g. 0, Tau, Tau/2 or Tau/4
)

// SSGrowth selects the growth strategy for slow-start.
type SSGrowth int

const (
	SSGrowthNoABC  = iota // grow by one MSS per ACK
	SSGrowthABC2          // use ABC with base of 2, grow by acked bytes
	SSGrowthABC1_5        // use ABC with base of 1.5, grow by 1/2 acked bytes
)

// Slow-Start: ESSP params
const (
	EsspHalfKExit      = true // if true, exit earlier, at K(i*2) instead of K(i)
	EsspDelayThreshold = 1.25 // if > 1, advance stage when sRTT > x * minRTT
	EsspCWNDTargeting  = true // if true, target CWND on advance
	EsspCENoResponse   = true // if true, skip normal response to CE
	EsspSCENoResponse  = true // if true, skip normal response to SCE
)

// Sender: TCP params
const (
	MSS      = Bytes(1500)
	IW       = 10 * MSS
	RTTAlpha = 0.125 // RFC 6298
)

// Sender: CUBIC params
const (
	CubicBeta            = 0.7  // RFC 9438 Section 4.6
	CubicC               = 0.4  // RFC 9438 Section 5
	CubicFastConvergence = true // RFC 9438 Section 4.7
)

// Sender: Scalable params
const (
	ScalableCEMD             = 0.7   // RFC 8511
	ScalableRenoFloor        = false // if true, grow at least by Reno-linear
	ScalableNoGrowthOnSignal = true  // if true, do not grow on ECE or ESCE
	ScalableCWNDTargetingCE  = false // if true, do targeting on CE
)

// Sender: MASLO params
const (
	MasloBeta               = 0.85 // rate MD on CE
	MasloM                  = 200  // expected steady-state marking frequency, in Hz
	MasloOrttAdjustment     = true // if true, adjust oRTT on CE marks
	MasloProbeThreshold     = 2.0  // multiple of pacing rate since last signal
	MasloCwndScaleFactor    = 1.25 // scale factor for pacing rate to CWND
	MasloBandwidthProbing   = true // if true, enable bandwidth probing
	MasloSCEMDApproximation = true // if true, approximate SCE response (default)
)

// Sender: pacing params
const (
	DefaultPacingSSRatio = 1.0 // Linux default == 2.0
	DefaultPacingCARatio = 1.0 // Linux default == 1.2
)

// Sender: HyStart++ (RFC 9406)
const (
	HyMinRTTThresh     = Clock(4 * time.Millisecond)  // default 4ms
	HyMaxRTTThresh     = Clock(16 * time.Millisecond) // default 16ms
	HyMinRTTDivisor    = 8                            // default 8
	HyNRTTSample       = 8                            // default 8
	HyCSSGrowthDivisor = 4                            // default 4
	HyCSSRounds        = 5                            // default 5
	HyStartLNoPacing   = 8                            // default 8
)

// Slow Start
var (
	Slick1  = NewSlick(Clock(1 * time.Millisecond))
	Slick2  = NewSlick(Clock(2 * time.Millisecond))
	Slick3  = NewSlick(Clock(3 * time.Millisecond))
	Slick4  = NewSlick(Clock(4 * time.Millisecond))
	Slick5  = NewSlick(Clock(5 * time.Millisecond))
	Slick10 = NewSlick(Clock(10 * time.Millisecond))
)

// Iface: AQM queue length restriction at which panic occurs
const IfaceHardQueueLen = 1000000

// AQM
const (
	JitterCompensation = true
)

// main
const (
	ProfileCPU    = false
	ProfileMemory = false
)
