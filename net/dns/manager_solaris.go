// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package dns

import (
	"tailscale.com/health"
	"tailscale.com/types/logger"
)

func NewOSConfigurator(logf logger.Logf, health *health.Tracker, iface string) (OSConfigurator, error) {
	return newDirectManager(logf, health), nil
}
