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
}

// newAqmPlot returns a new DelticMDS.
func newAqmPlot() aqmPlot {
	return aqmPlot{
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
	}
}

// Start implements Starter.
func (a *aqmPlot) Start(node Node) (err error) {
	if PlotMarks {
		if err = a.marksPlot.Open("marks.xpl"); err != nil {
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
