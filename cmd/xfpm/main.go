/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"github.com/Nehonix-Team/XFMP/cmd/xfpm/cmd"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

func main() {
	// Register OS signal handler at the very first moment.
	// This ensures SIGINT/SIGTERM are captured before any Cobra command
	// or runtime default handler can intercept them.
	utils.SignalManager.Start()
	cmd.Execute()
}
