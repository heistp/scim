// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"time"
)

// Sender: test duration
const Duration = 20 * time.Second

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

// Sender: plots
const (
	PlotInFlight = false
	PlotCwnd     = true
	PlotRTT      = false
)

// Sender: flows
var Flows = []Flow{
	AddFlow(true),
	//AddFlow(true),
}

// Iface: plots
const (
	PlotSojourn = true
	PlotMarks   = false
)

// IFace: rate and rate schedule
var Rate = 100 * Mbps
var RateSchedule = []RateAt{
	//RateAt{Clock(10 * time.Second), 200 * Mbps},
}

// Iface: AQM config
// var UseAQM = NewDelmin1(Clock(5 * time.Millisecond))
var UseAQM = NewDelmin2(Clock(5000*time.Microsecond),
	Clock(100*time.Microsecond))

// var UseAQM = NewRamp()
var SCERampMin = Clock(TransferTime(Rate, Bytes(MSS))) * 1

const SCERampMax = Clock(100 * time.Millisecond)

// Delay: delays for each flow
var FlowDelay = []Clock{
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
	Clock(20 * time.Millisecond),
}

// Receiver
const (
	PlotGoodput = true
)

// main
const Profile = false
