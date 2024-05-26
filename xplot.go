// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"bufio"
	"fmt"
	"os"
	"text/template"
)

const xplotHeader = `{{.X.Type}} {{.Y.Type}}
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
	Type  string
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

func (p *Xplot) Dot(x any, y any, color int) {
	fmt.Fprintf(p.writer, "dot %s %s %d\n", x, y, color)
}

func (p *Xplot) PlotX(x any, y any, color int) {
	fmt.Fprintf(p.writer, "x %s %s %d\n", x, y, color)
}

func (p *Xplot) Close() error {
	fmt.Fprintf(p.writer, "go\n")
	p.writer.Flush()
	return p.file.Close()
}
