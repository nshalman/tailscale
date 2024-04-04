// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package dns

import (
	"tailscale.com/types/logger"
)

func NewOSConfigurator(logf logger.Logf, _ string) (OSConfigurator, error) {
	return newDirectManager(logf), nil
}
