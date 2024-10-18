// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"fmt"
	"strconv"
	"time"
)

// aqmPlot makes plots for AQM algorithms.
type aqmPlot struct {
	propPlot   Xplot
	noSCE      int
	noCE       int
	noDrop     int
	freqPlot   Xplot
	priorSCE   Clock
	priorCE    Clock
	priorDrop  Clock
	emitSigCtr int
	sojourn    Xplot
	adjSojourn Xplot
	qlen       Xplot
	deltaSigma Xplot
	byteSec    Xplot
}

// newAqmPlot returns a new DelticMDS.
func newAqmPlot() *aqmPlot {
	return &aqmPlot{
		Xplot{
			Title: "Mark Proportion - SCE:white, CE:yellow, drop:red",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Proportion",
			},
		}, // propPlot
		0, // noSCE
		0, // noCE
		0, // noDrop
		Xplot{
			Title: "Mark Frequency - SCE:white, CE:yellow, drop:red",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Frequency (Hz)",
			},
		}, // freqPlot
		0, // priorSCE
		0, // priorCE
		0, // priorDrop
		0, // emitSigCtr
		Xplot{
			Title: "Queue Sojourn Time (red: queue empty after dequeue)",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Sojourn time (ms)",
			},
			Decimation: PlotSojournInterval,
		}, // sojourn
		Xplot{
			Title: "Queue Adjusted Sojourn Time",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Adj. sojourn time (ms)",
			},
			Decimation: PlotAdjSojournInterval,
		}, // sojourn
		Xplot{
			Title: "Queue Length",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Length (packets)",
			},
			Decimation: PlotQueueLengthInterval,
		}, // qlen
		Xplot{
			Title: "Delta-Sigma - delta:red, sigma:yellow",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Value",
			},
			NonzeroAxis: true,
		}, // deltaSigma
		Xplot{
			Title: "Queue Byte-Seconds",
			X: Axis{
				Label: "Time (S)",
			},
			Y: Axis{
				Label: "Byte-Seconds",
			},
			Decimation: PlotByteSecondsInterval,
		}, // byteSec
	}
}

// Start implements Starter.
func (a *aqmPlot) Start(node Node) (err error) {
	if PlotMarkProportion {
		if err = a.propPlot.Open("mark-proportion.xpl"); err != nil {
			return
		}
	}
	if PlotMarkFrequency {
		if err = a.freqPlot.Open("mark-frequency.xpl"); err != nil {
			return
		}
	}
	if PlotSojourn {
		if err = a.sojourn.Open("sojourn.xpl"); err != nil {
			return
		}
	}
	if PlotAdjSojourn {
		if err = a.adjSojourn.Open("adj-sojourn.xpl"); err != nil {
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
	if PlotByteSeconds {
		if err = a.byteSec.Open("queue-bytesec.xpl"); err != nil {
			return
		}
	}
	return nil
}

// Stop implements Stopper.
func (a *aqmPlot) Stop(node Node) error {
	if PlotMarkProportion {
		a.propPlot.Close()
	}
	if PlotMarkFrequency {
		a.freqPlot.Close()
	}
	if PlotSojourn {
		a.sojourn.Close()
	}
	if PlotAdjSojourn {
		a.adjSojourn.Close()
	}
	if PlotQueueLength {
		a.qlen.Close()
	}
	if PlotDeltaSigma {
		a.deltaSigma.Close()
	}
	if PlotByteSeconds {
		a.byteSec.Close()
	}
	if EmitMark && a.emitSigCtr != 0 {
		fmt.Println()
	}
	return nil
}

// plotMark plots and emits the given mark, as configured.
func (a *aqmPlot) plotMark(m mark, now Clock) {
	if PlotMarkProportion {
		switch m {
		case markNone:
			a.noSCE++
			a.noCE++
			a.noDrop++
		case markSCE:
			p := 1.0 / float64(a.noSCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.propPlot.Dot(now, ps, colorWhite)
			a.noSCE = 0
			a.noCE++
			a.noDrop++
		case markCE:
			p := 1.0 / float64(a.noCE+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.propPlot.PlotX(now, ps, colorYellow)
			a.noCE = 0
			a.noSCE++
			a.noDrop++
		case markDrop:
			p := 1.0 / float64(a.noDrop+1)
			ps := strconv.FormatFloat(p, 'f', -1, 64)
			a.propPlot.PlotX(now, ps, colorRed)
			a.noDrop = 0
			a.noCE++
			a.noSCE++
		}
	}
	if PlotMarkFrequency {
		switch m {
		case markNone:
		case markSCE:
			f := 1.0 / float64(time.Duration(now-a.priorSCE).Seconds())
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			a.freqPlot.Dot(now, fs, colorWhite)
			a.priorSCE = now
		case markCE:
			f := 1.0 / float64(time.Duration(now-a.priorCE).Seconds())
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			a.freqPlot.PlotX(now, fs, colorYellow)
			a.priorCE = now
		case markDrop:
			f := 1.0 / float64(time.Duration(now-a.priorDrop).Seconds())
			fs := strconv.FormatFloat(f, 'f', -1, 64)
			a.freqPlot.PlotX(now, fs, colorRed)
			a.priorDrop = now
		}
	}
	if EmitMark {
		a.emitMark(m)
	}
}

// plotByteSeconds plots a byte-seconds value.
func (a *aqmPlot) plotByteSeconds(byteSec float64, now Clock) {
	bs := strconv.FormatFloat(byteSec, 'f', -1, 64)
	a.byteSec.Dot(now, bs, colorWhite)
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
	a.emitSigCtr++
	if a.emitSigCtr == 64 {
		fmt.Println()
		a.emitSigCtr = 0
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

// plotAdjSojourn plots the adjusted sojourn time.
func (a *aqmPlot) plotAdjSojourn(sojourn Clock, empty bool, now Clock) {
	if PlotAdjSojourn {
		c := colorWhite
		if empty {
			c = colorRed
		}
		a.adjSojourn.Dot(now, sojourn.StringMS(), c)
	}
}

// plotDeltaSigma plots the delta, sigma and accumulator/1000 values.
func (a *aqmPlot) plotDeltaSigma(delta Clock, sigma Clock, now Clock) {
	if PlotDeltaSigma {
		f := a.deltaSigma.Dot
		//if !mark {
		//	f = a.deltaSigma.PlotX
		//}
		f(now, strconv.FormatInt(int64(delta), 10), colorRed)
		f(now, strconv.FormatInt(int64(sigma), 10), colorYellow)
		//f(now, strconv.FormatInt(int64(acc/1000), 10), colorWhite)
	}
}
