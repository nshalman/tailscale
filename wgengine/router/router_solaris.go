// Copyright (c) 2022 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"strings"

	"golang.zx2c4.com/wireguard/tun"
	"tailscale.com/types/logger"
	"tailscale.com/wgengine/monitor"
)

// For now this router only supports the userspace WireGuard implementations.

func newUserspaceRouter(logf logger.Logf, tundev tun.Device, linkMon *monitor.Mon) (Router, error) {
	return newUserspaceSunosRouter(logf, tundev, linkMon)
}

func cleanup(logf logger.Logf, interfaceName string) {
	ipadm := []string{"ipadm", "show-addr", "-p", "-o", "addrobj"}
	out, err := cmd(ipadm...).Output()
	if err != nil {
		logf("ipadm show-addr: %v\n%s", err, out)
	}
	for _, a := range strings.Fields(string(out)) {
		s := strings.Split(a, "/")
		if len(s) > 1 && strings.Contains(s[1], "tailscale") {
			ipadm = []string{"ipadm", "down-addr", "-t", a}
			cmdVerbose(logf, ipadm)
			ipadm = []string{"ipadm", "delete-addr", a}
			cmdVerbose(logf, ipadm)
			ipadm = []string{"ipadm", "delete-if", s[0]}
			cmdVerbose(logf, ipadm)
		}
	}
	ifcfg := []string{"ifconfig", interfaceName, "unplumb"}
	if out, err := cmd(ifcfg...).CombinedOutput(); err != nil {
		logf("ifconfig unplumb: %v\n%s", err, out)
	}
	ifcfg = []string{"ifconfig", interfaceName, "inet6", "unplumb"}
	if out, err := cmd(ifcfg...).CombinedOutput(); err != nil {
		logf("ifconfig inet6 unplumb: %v\n%s", err, out)
	}
}
