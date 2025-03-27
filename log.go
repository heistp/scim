// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright 2025 Pete Heist

package main

import (
	"fmt"
	"log"
)

// logf logs a message.
func logf(now Clock, id nodeID, format string, a ...any) {
	log.Printf("%s [%d]: %s", now, id, fmt.Sprintf(format, a...))
}
