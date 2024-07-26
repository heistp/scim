// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"bufio"
	"fmt"
	"os"
	"text/template"
)

const xplotHeader = `double double
title
{{.Title}}
{{if .X.Label -}}
xlabel
{{.X.Label}}
{{end -}}
{{if .Y.Label -}}
ylabel
{{.Y.Label}}
{{end -}}
{{if .X.Units -}}
xunits
{{.X.Units}}
{{end -}}
{{if .Y.Units -}}
yunits
{{.Y.Units}}
{{end -}}
{{if not .NonzeroAxis -}}
invisible 0 0
{{end -}}
`

type Axis struct {
	Label string
	Units string
}

type Xplot struct {
	Title       string
	X           Axis
	Y           Axis
	NonzeroAxis bool
	Decimation  Clock
	file        *os.File
	writer      *bufio.Writer
	prior       map[int]Clock
}

type symbology int

const (
	symbologyDot symbology = (iota + 1) * 1024
	symbologyPlus
	symbologyX
)

type color int

const (
	colorWhite color = iota
	colorGreen
	colorRed
	colorBlue
	colorYellow
	colorPurple
	colorOrange
	colorMagenta
	colorPink
)

func (p *Xplot) Open(name string) (err error) {
	var t *template.Template
	if t, err = template.New("XplotHeader").Parse(xplotHeader); err != nil {
		return
	}
	if p.file, err = os.Create(name); err != nil {
		return
	}
	p.writer = bufio.NewWriter(p.file)
	p.prior = make(map[int]Clock)
	err = t.Execute(p.writer, p)
	return
}

func (p *Xplot) Dot(now Clock, y any, color color) {
	if !p.decimate(now, symbologyDot, color) {
		fmt.Fprintf(p.writer, "dot %s %s %d\n", now, y, color)
	}
}

func (p *Xplot) Plus(now Clock, y any, color color) {
	if !p.decimate(now, symbologyPlus, color) {
		fmt.Fprintf(p.writer, "+ %s %s %d\n", now, y, color)
	}
}

func (p *Xplot) PlotX(now Clock, y any, color color) {
	if !p.decimate(now, symbologyX, color) {
		fmt.Fprintf(p.writer, "x %s %s %d\n", now, y, color)
	}
}

func (p *Xplot) Line(x0, y0, x1, y1 any, color color) {
	fmt.Fprintf(p.writer, "line %s %s %s %s %d\n", x0, y0, x1, y1, color)
}

// decimate returns true if the given symbology and color may be plotted now.
func (p *Xplot) decimate(now Clock, sym symbology, color color) bool {
	i := int(sym) * int(color)
	var ok bool
	var c Clock
	if c, ok = p.prior[i]; !ok || now-c > p.Decimation {
		p.prior[i] = now
		return false
	}
	return true
}

func (p *Xplot) Close() error {
	fmt.Fprintf(p.writer, "go\n")
	p.writer.Flush()
	return p.file.Close()
}
