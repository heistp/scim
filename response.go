// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
	"time"
)

// A Responder adjusts cwnd in response to a congestion control signal or event.
type Responder interface {
	Respond(flow *Flow, node Node) (cnwd Bytes)
}

// SCE_MD is the multiplicative decrease for the SCE MD-Scaling response.
var SCE_MD = math.Pow(CEMD, 1.0/Tau)

// MD is a generic multiplicative decrease Responder.
type MD float64

// Respond implements Responder.
func (m MD) Respond(flow *Flow, node Node) (cwnd Bytes) {
	cwnd = Bytes(float64(flow.cwnd) * float64(m))
	return
}

// RateFairMD is a Responder that performs an MD-Scaling multiplicative decrease
// that results in rate independent fairness with other MD-Scaling flows.
type RateFairMD struct {
	MD         float64
	NominalRTT Clock
}

// Respond implements Responder.
func (r RateFairMD) Respond(flow *Flow, node Node) (cwnd Bytes) {
	t := float64(Tau) * float64(flow.srtt) * float64(flow.srtt) /
		float64(r.NominalRTT) / float64(r.NominalRTT)
	m := math.Pow(r.MD, float64(1)/t)
	cwnd = Bytes(float64(flow.cwnd) * m)
	return
}

// HybridFairMD is a Responder that performs an MD-Scaling multiplicative
// decrease that is between rate independent fairness and cwnd convergence with
// other MD-Scaling flows.
type HybridFairMD struct {
	MD         float64
	NominalRTT Clock
}

// Respond implements Responder.
func (h HybridFairMD) Respond(flow *Flow, node Node) (cwnd Bytes) {
	t := float64(Tau) * float64(flow.srtt) / float64(h.NominalRTT)
	m := math.Pow(h.MD, float64(1)/t)
	cwnd = Bytes(float64(flow.cwnd) * m)
	return
}

// SqrtP is a 1/sqrt(p) Responder.
type SqrtP struct {
}

// Respond implements Responder.
func (s SqrtP) Respond(flow *Flow, node Node) (cwnd Bytes) {
	m := 1.0 - math.Sqrt(float64(flow.cwnd))/float64(flow.cwnd)
	cwnd = Bytes(float64(flow.cwnd) * m)
	return
}

// TargetCWND responds by using CWND targeting
// cwnd = (FlightSize_SRTTBefore * minRTT / SRTT).
type TargetCWND struct {
}

// Respond implements Responder.
func (TargetCWND) Respond(flow *Flow, node Node) (cwnd Bytes) {
	cwnd0 := flow.cwnd
	flight := flow.inFlightWindow.at(node.Now() - flow.srtt)
	cwnd = flight * Bytes(flow.minRtt) / Bytes(flow.srtt)
	node.Logf("target cwnd:%d cwnd0:%d flight:%d minRtt:%.2fms srtt:%.2fms",
		cwnd, cwnd0, flight,
		time.Duration(flow.minRtt).Seconds()*1000,
		time.Duration(flow.srtt).Seconds()*1000)
	return
}

// TargetResponse responds by using CWND targeting followed by a regular SCE
// response.
type TargetResponse struct {
}

// Respond implements Responder.
func (TargetResponse) Respond(flow *Flow, node Node) (cwnd Bytes) {
	//cwnd0 := flow.cwnd
	flight := flow.inFlightWindow.at(node.Now() - flow.srtt)
	//cwnd = flight * Bytes(flow.minRtt+flow.srtt) / Bytes(2*flow.srtt)
	cwnd = flight * Bytes(flow.minRtt) / Bytes(flow.srtt)
	m := 1.0 - math.Sqrt(float64(cwnd))/float64(cwnd)
	cwnd = Bytes(float64(cwnd) * m)
	//cwnd = Bytes(float64(cwnd) * float64(SCE_MD))
	//node.Logf("approach cwnd:%d cwnd0:%d flight:%d minRtt:%.2fms srtt:%.2fms",
	//	cwnd, cwnd0, flight,
	//	time.Duration(flow.minRtt).Seconds()*1000,
	//	time.Duration(flow.srtt).Seconds()*1000)
	return
}

// HalfCWND responds by dividing CWND by 2.
type HalfCWND struct {
}

// Respond implements Responder.
func (HalfCWND) Respond(flow *Flow, node Node) (cwnd Bytes) {
	cwnd = flow.cwnd / 2
	return
}

// NoResponse is a Responder that does nothing.
type NoResponse struct {
}

// Respond implements Responder.
func (NoResponse) Respond(flow *Flow, node Node) (cwnd Bytes) {
	cwnd = flow.cwnd
	return
}
