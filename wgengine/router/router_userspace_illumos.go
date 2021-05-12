// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a Illumos-style
// license that can be found in the LICENSE file.

// +build illumos

package router

import (
	"fmt"
	"log"
	"os/exec"

	"golang.zx2c4.com/wireguard/tun"
	"inet.af/netaddr"
	"tailscale.com/types/logger"
)

type userspaceIllumosRouter struct {
	logf    logger.Logf
	tunname string
	local   []netaddr.IPPrefix
	routes  map[netaddr.IPPrefix]struct{}
}

func newUserspaceIllumosRouter(logf logger.Logf, tundev tun.Device) (Router, error) {
	tunname, err := tundev.Name()
	if err != nil {
		return nil, err
	}

	return &userspaceIllumosRouter{
		logf:    logf,
		tunname: tunname,
	}, nil
}

func (r *userspaceIllumosRouter) addrsToRemove(newLocalAddrs []netaddr.IPPrefix) (remove []netaddr.IPPrefix) {
	for _, cur := range r.local {
		found := false
		for _, v := range newLocalAddrs {
			found = (v == cur)
			if found {
				break
			}
		}
		if !found {
			remove = append(remove, cur)
		}
	}
	return
}

func (r *userspaceIllumosRouter) addrsToAdd(newLocalAddrs []netaddr.IPPrefix) (add []netaddr.IPPrefix) {
	for _, cur := range newLocalAddrs {
		found := false
		for _, v := range r.local {
			found = (v == cur)
			if found {
				break
			}
		}
		if !found {
			add = append(add, cur)
		}
	}
	return
}

func cmd(args ...string) *exec.Cmd {
	if len(args) == 0 {
		log.Fatalf("exec.Cmd(%#v) invalid; need argv[0]", args)
	}
	return exec.Command(args[0], args[1:]...)
}

func (r *userspaceIllumosRouter) Up() error {
	ifup := []string{"ifconfig", r.tunname, "up"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		r.logf("running ifconfig failed: %v\n%s", err, out)
		// this seems to fail harmlessly on illumos
		//return err
	}
	return nil
}

func inet(p netaddr.IPPrefix) string {
	if p.IP().Is6() {
		return "inet6"
	}
	return "inet"
}

func (r *userspaceIllumosRouter) Set(cfg *Config) (reterr error) {
	if cfg == nil {
		cfg = &shutdownConfig
	}

	var errq error
	setErr := func(err error) {
		if errq == nil {
			errq = err
		}
	}

	// illumos requires routes to have a nexthop. For routes such as
	// ours where the nexthop is meaningless, you're supposed to use
	// one of the local IP addresses of the interface. Find an IPv4
	// and IPv6 address we can use for this purpose.
	var firstGateway4 string
	var firstGateway6 string
	for _, addr := range cfg.LocalAddrs {
		if addr.IP().Is4() && firstGateway4 == "" {
			firstGateway4 = addr.IP().String()
		} else if addr.IP().Is6() && firstGateway6 == "" {
			firstGateway6 = addr.IP().String()
		}
	}

	// Update the addresses. TODO(nshalman)
	for _, addr := range r.addrsToRemove(cfg.LocalAddrs) {
		arg := []string{"ifconfig", r.tunname, inet(addr), addr.String(), "-alias"}
		out, err := cmd(arg...).CombinedOutput()
		if err != nil {
			r.logf("addr del failed: %v => %v\n%s", arg, err, out)
			setErr(err)
		}
	}
	for _, addr := range r.addrsToAdd(cfg.LocalAddrs) {
		addrString := fmt.Sprintf("local=%s,remote=%s", addr.String(), addr.IP().String())
		var arg = []string{"ipadm", "create-addr", "-t", "-T", "static", "-a", addrString, r.tunname + "/" + inet(addr)}
		out, err := cmd(arg...).CombinedOutput()
		if err != nil {
			r.logf("addr add failed: %v => %v\n%s", arg, err, out)
			setErr(err)
		}
		var arg2 = []string{"ifconfig"}
		out, err = cmd(arg2...).CombinedOutput()
		r.logf("%v => %v\n%s", arg, err, out)
	}

	newRoutes := make(map[netaddr.IPPrefix]struct{})
	for _, route := range cfg.Routes {
		newRoutes[route] = struct{}{}
	}
	// Delete any pre-existing routes. TODO(nshalman)
	for route := range r.routes {
		if _, keep := newRoutes[route]; !keep {
			net := route.IPNet()
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			del := "del"
			routedel := []string{"route", "-q", "-n",
				del, "-" + inet(route), nstr,
				"-iface", r.tunname}
			out, err := cmd(routedel...).CombinedOutput()
			if err != nil {
				r.logf("route del failed: %v: %v\n%s", routedel, err, out)
				setErr(err)
			}
		}
	}
	for route := range newRoutes {
		if _, exists := r.routes[route]; !exists {
			net := route.IPNet()
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			var gateway string
			if route.IP().Is4() && firstGateway4 != "" {
				gateway = firstGateway4
			}
			if route.IP().Is6() && firstGateway6 != "" {
				gateway = firstGateway6
			}
			routeadd := []string{"route", "-q", "-n",
				"add", "-" + inet(route), nstr,
				"-ifp", r.tunname, gateway, "-iface"}
			out, err := cmd(routeadd...).CombinedOutput()
			if err != nil {
				r.logf("addr add failed: %v: %v\n%s", routeadd, err, out)
				setErr(err)
			}
		}
	}

	// Store the interface and routes so we know what to change on an update.
	if errq == nil {
		r.local = append([]netaddr.IPPrefix{}, cfg.LocalAddrs...)
	}
	r.routes = newRoutes

	return errq
}

func (r *userspaceIllumosRouter) Close() error {
	cleanup(r.logf, r.tunname)
	return nil
}
