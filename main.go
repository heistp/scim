// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

func main() {
	log.SetFlags(0)
	if ProfileCPU {
		var f *os.File
		var e error
		if f, e = os.Create("scim-cpu.prof"); e != nil {
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
	if ProfileMemory {
		var f *os.File
		var e error
		if f, e = os.Create("scim-mem.prof"); e != nil {
			log.Fatal(e)
		}
		defer f.Close()
		runtime.GC()
		if e = pprof.WriteHeapProfile(f); e != nil {
			log.Fatal(e)
		}
	}
}
