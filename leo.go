// SPDX-License-Identifier: GPL-3.0
// Copyright 2024 Pete Heist

package main

import (
	"math"
)

// LeoStageMax is the maximum number of ESSP stages.
const LeoStageMax = 44

var (
	LeoK       [LeoStageMax*2 - 1]int // K for each stage (n+1 Leonardo numbers)
	EsspScale  [LeoStageMax]float64   // scale factors for each ESSP stage
	MasloSCEMD [LeoStageMax]float64   // Maslo SCE MD for each stage
)

func init() {
	s := 1.0
	a := 1
	b := 1
	for i := 0; i < len(LeoK); i++ {
		LeoK[i] = b
		s *= 1.0 + 1.0/float64(b)
		a, b = b, 1+a+b
	}
	for i := 0; i < len(EsspScale); i++ {
		EsspScale[i] = s
		x := 1.0 + 1.0/float64(LeoK[i])
		MasloSCEMD[i] = 1.0 / math.Pow(x, 1.0/16.0)
		s /= x
		//fmt.Printf("%d %d %d %.10f %.10f\n",
		//	i, LeoK[i], LeoK[i*2], EsspScale[i], MasloSCEMD[i])
	}
}
