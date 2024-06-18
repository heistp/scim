// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"log"
	"os"
	"runtime/pprof"
)

func main() {
	log.SetFlags(0)
	if Profile {
		var f *os.File
		var e error
		if f, e = os.Create("sceaimd.prof"); e != nil {
			log.Fatal(e)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	h := []Handler{
		NewSender(FlowSchedule),
		NewIface(RateInit, RateSchedule, UseAQM),
		Delay(FlowDelay),
		NewReceiver(),
	}
	s := NewSim(h)
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
