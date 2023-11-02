// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package router

import (
	"strings"

	"github.com/tailscale/wireguard-go/tun"
	"tailscale.com/types/logger"
	"tailscale.com/net/netmon"
)

// For now this router only supports the userspace WireGuard implementations.

func newUserspaceRouter(logf logger.Logf, tundev tun.Device, linkMon *netmon.Monitor) (Router, error) {
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
