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
	file        *os.File
	writer      *bufio.Writer
}

func (p *Xplot) Open(name string) (err error) {
	var t *template.Template
	if t, err = template.New("XplotHeader").Parse(xplotHeader); err != nil {
		return
	}
	if p.file, err = os.Create(name); err != nil {
		return
	}
	p.writer = bufio.NewWriter(p.file)
	err = t.Execute(p.writer, p)
	return
}

func (p *Xplot) Dot(x, y any, color int) {
	fmt.Fprintf(p.writer, "dot %s %s %d\n", x, y, color)
}

func (p *Xplot) Plus(x, y any, color int) {
	fmt.Fprintf(p.writer, "+ %s %s %d\n", x, y, color)
}

func (p *Xplot) PlotX(x, y any, color int) {
	fmt.Fprintf(p.writer, "x %s %s %d\n", x, y, color)
}

func (p *Xplot) Line(x0, y0, x1, y1 any, color int) {
	fmt.Fprintf(p.writer, "line %s %s %s %s %d\n", x0, y0, x1, y1, color)
}

func (p *Xplot) Close() error {
	fmt.Fprintf(p.writer, "go\n")
	p.writer.Flush()
	return p.file.Close()
}
