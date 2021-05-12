// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tailscale/wireguard-go/tun"
	"tailscale.com/types/logger"
)

// For now this router only supports the userspace WireGuard implementations.

func newUserspaceRouter(logf logger.Logf, tundev tun.Device) (Router, error) {
	return newUserspaceIllumosRouter(logf, tundev)
}

func cleanup(logf logger.Logf, interfaceName string) {
	ifcfg := []string{"ifconfig", interfaceName, "unplumb"}
	if out, err := cmd(ifcfg...).CombinedOutput(); err != nil {
		logf("ifconfig unplumb: %v\n%s", err, out)
	}
	ifcfg = []string{"ifconfig", interfaceName, "inet6", "unplumb"}
	if out, err := cmd(ifcfg...).CombinedOutput(); err != nil {
		logf("ifconfig inet6 unplumb: %v\n%s", err, out)
	}
}
