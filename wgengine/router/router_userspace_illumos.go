// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a Illumos-style
// license that can be found in the LICENSE file.

// +build illumos

package router

import (
	"errors"
	"fmt"

	"github.com/tailscale/wireguard-go/device"
	"github.com/tailscale/wireguard-go/tun"
	"inet.af/netaddr"
	"tailscale.com/types/logger"
	"tailscale.com/version"
	"tailscale.com/wgengine/router/dns"
)

type userspaceIllumosRouter struct {
	logf    logger.Logf
	tunname string
	local   netaddr.IPPrefix
	routes  map[netaddr.IPPrefix]struct{}

	dns *dns.Manager
}

func newUserspaceIllumosRouter(logf logger.Logf, _ *device.Device, tundev tun.Device) (Router, error) {
	tunname, err := tundev.Name()
	if err != nil {
		return nil, err
	}

	mconfig := dns.ManagerConfig{
		Logf:          logf,
		InterfaceName: tunname,
	}

	return &userspaceIllumosRouter{
		logf:    logf,
		tunname: tunname,
		dns:     dns.NewManager(mconfig),
	}, nil
}

func (r *userspaceIllumosRouter) Up() error {
/*
	ifup := []string{"ifconfig", r.tunname, "up"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		r.logf("running ifconfig failed: %v\n%s", err, out)
		return err
	}
*/
	return nil
}

func (r *userspaceIllumosRouter) Set(cfg *Config) error {
	if cfg == nil {
		cfg = &shutdownConfig
	}
	if len(cfg.LocalAddrs) == 0 {
		return nil
	}
	// TODO: support configuring multiple local addrs on interface.
	if len(cfg.LocalAddrs) != 1 {
		return errors.New("freebsd doesn't support setting multiple local addrs yet")
	}
	localAddr := cfg.LocalAddrs[0]

	var errq error

	// Update the address.
	if localAddr != r.local {
		// If the interface is already set, remove it.
		if r.local != (netaddr.IPPrefix{}) {
			addrdel := []string{"ifconfig", r.tunname,
				"inet", r.local.String(), "-alias"}
			out, err := cmd(addrdel...).CombinedOutput()
			if err != nil {
				r.logf("addr del failed: %v: %v\n%s", addrdel, err, out)
				if errq == nil {
					errq = err
				}
			}
		}

		// Add the interface.
		addradd := []string{"ifconfig", r.tunname,
			"inet", localAddr.String(), localAddr.IP.String()}
		out, err := cmd(addradd...).CombinedOutput()
		if err != nil {
			r.logf("addr add failed: %v: %v\n%s", addradd, err, out)
			if errq == nil {
				errq = err
			}
		}
	}

	newRoutes := make(map[netaddr.IPPrefix]struct{})
	for _, route := range cfg.Routes {
		newRoutes[route] = struct{}{}
	}
	// Delete any pre-existing routes.
	for route := range r.routes {
		if _, keep := newRoutes[route]; !keep {
			net := route.IPNet()
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits)
			del := "del"
			if version.OS() == "macOS" {
				del = "delete"
			}
			routedel := []string{"route", "-q", "-n",
				del, "-inet", nstr,
				localAddr.IP.String(), "-iface"}
			out, err := cmd(routedel...).CombinedOutput()
			if err != nil {
				r.logf("route del failed: %v: %v\n%s", routedel, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}
	// Add the routes.
	for route := range newRoutes {
		if _, exists := r.routes[route]; !exists {
			net := route.IPNet()
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits)
			routeadd := []string{"route", "-q", "-n",
				"add", nstr,
				localAddr.IP.String(), "-iface"}
			out, err := cmd(routeadd...).CombinedOutput()
			if err != nil {
				r.logf("addr add failed: %v: %v\n%s", routeadd, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}

	// Bring up the interface
	ifup := []string{"ifconfig", r.tunname, "up"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		r.logf("running ifconfig failed: %v\n%s", err, out)
		return err
	}
	return nil

	// Store the interface and routes so we know what to change on an update.
	r.local = localAddr
	r.routes = newRoutes

	if err := r.dns.Set(cfg.DNS); err != nil {
		errq = fmt.Errorf("dns set: %v", err)
	}

	return errq
}

func (r *userspaceIllumosRouter) Close() error {
	if err := r.dns.Down(); err != nil {
		r.logf("dns down: %v", err)
	}
	// No interface cleanup is necessary during normal shutdown.
	return nil
}
