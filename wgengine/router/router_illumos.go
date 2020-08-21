// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"github.com/tailscale/wireguard-go/device"
	"github.com/tailscale/wireguard-go/tun"
	"tailscale.com/types/logger"
)

// For now this router only supports the userspace WireGuard implementations.

func newUserspaceRouter(logf logger.Logf, _ *device.Device, tundev tun.Device) (Router, error) {
	return newUserspaceIllumosRouter(logf, nil, tundev)
}

func cleanup(logf logger.Logf, interfaceName string) {
	// If the interface was left behind, ifconfig down will not remove it.
	// In fact, this will leave a system in a tainted state where starting tailscaled
	// will result in "interface tailscale0 already exists"
	// until the defunct interface is ifconfig-destroyed.
	ifup := []string{"ifconfig", interfaceName, "down"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		logf("ifconfig down: %v\n%s", err, out)
	}
	ifup = []string{"ifconfig", interfaceName, "unplumb"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		logf("ifconfig unplumb: %v\n%s", err, out)
	}
}
