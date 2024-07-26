// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"strconv"
)

// aqmPlot makes plots for AQM algorithms.
type aqmPlot struct {
	marksPlot    Xplot
	noSCE        int
	noCE         int
	noDrop       int
	emitMarksCtr int
	sojourn      Xplot
	qlen         Xplot
	deltaSigma   Xplot
}

// newAqmPlot returns a new DelticMDS.
func newAqmPlot() *aqmPlot {
	return &aqmPlot{
		Xplot{
			Title: "Congestion Signals - SCE:white, CE:yellow, drop:red",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Proportion",
			},
		}, // marksPlot
		0, // noSCE
		0, // noCE
		0, // noDrop
		0, // emitMarksCtr
		Xplot{
			Title: "Queue Sojourn Time",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Sojourn time (ms)",
			},
			Decimation: PlotSojournInterval,
		},
		Xplot{
			Title: "Queue Length",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Length (packets)",
			},
			Decimation: PlotQueueLengthInterval,
		},
		Xplot{
			Title: "Delta-Sigma - delta:red, sigma:yellow, acc/1000:white",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Value",
			},
			NonzeroAxis: true,
		},
	}
}

// Start implements Starter.
func (a *aqmPlot) Start(node Node) (err error) {
	if PlotMarks {
		if err = a.marksPlot.Open("marks.xpl"); err != nil {
			return
		}
	}
	if PlotSojourn {
		if err = a.sojourn.Open("sojourn.xpl"); err != nil {
			return
		}
	}
	if PlotQueueLength {
		if err = a.qlen.Open("queue-length.xpl"); err != nil {
			return
		}
	}
	if PlotDeltaSigma {
		if err = a.deltaSigma.Open("delta-sigma.xpl"); err != nil {
			return
		}
	}
	return nil
}

// Stop implements Stopper.
func (a *aqmPlot) Stop(node Node) error {
	if PlotMarks {
		a.marksPlot.Close()
	}
	if PlotSojourn {
		a.sojourn.Close()
	}
	if PlotQueueLength {
		a.qlen.Close()
	}
	if PlotDeltaSigma {
		a.deltaSigma.Close()
	}
	if EmitMarks && a.emitMarksCtr != 0 {
		fmt.Println()
	}
	return nil
}

// plotMark plots and emits the given mark, as configured.
func (a *aqmPlot) plotMark(m mark, now Clock) {
	if PlotMarks {
		switch m {
		case markNone:
			a.noSCE++
			a.noCE++
			a.noDrop++
		case markSCE:
			p := 1.0 / float64(a.noSCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.marksPlot.Dot(now, ps, 0)
			a.noSCE = 0
			a.noCE++
			a.noDrop++
		case markCE:
			p := 1.0 / float64(a.noCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.marksPlot.PlotX(now, ps, 4)
			a.noCE = 0
			a.noSCE++
			a.noDrop++
		case markDrop:
			p := 1.0 / float64(a.noDrop+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.marksPlot.PlotX(now, ps, 2)
			a.noDrop = 0
			a.noCE++
			a.noSCE++
		}
	}
	if EmitMarks {
		a.emitMark(m)
	}
}

// emitMark prints marks as characters.
func (a *aqmPlot) emitMark(m mark) {
	// emit marks as characters
	switch m {
	case markSCE:
		fmt.Print("s")
	case markCE:
		fmt.Print("c")
	case markDrop:
		fmt.Print("D")
	default:
		return
	}
	a.emitMarksCtr++
	if a.emitMarksCtr == 64 {
		fmt.Println()
		a.emitMarksCtr = 0
	}
}

// plotLength plots the queue length, in packets.
func (a *aqmPlot) plotLength(length int, now Clock) {
	if PlotQueueLength {
		c := colorWhite
		if length == 0 {
			c = colorRed
		}
		a.qlen.Dot(now, strconv.Itoa(length), c)
	}
}

// plotSojourn plots the sojourn time.
func (a *aqmPlot) plotSojourn(sojourn Clock, empty bool, now Clock) {
	if PlotSojourn {
		c := colorWhite
		if empty {
			c = colorRed
		}
		a.sojourn.Dot(now, sojourn.StringMS(), c)
	}
}

// plotDeltaSigma plots the delta, sigma and accumulator/1000 values.
func (a *aqmPlot) plotDeltaSigma(delta Clock, sigma Clock,
	acc Clock, now Clock) {
	if PlotDeltaSigma {
		f := a.deltaSigma.Dot
		//if !mark {
		//	f = a.deltaSigma.PlotX
		//}
		f(now, strconv.FormatInt(int64(delta), 10), colorRed)
		f(now, strconv.FormatInt(int64(sigma), 10), colorYellow)
		f(now, strconv.FormatInt(int64(acc/1000), 10), colorWhite)
	}
}
