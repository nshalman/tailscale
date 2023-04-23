// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

//go:build illumos || solaris
// +build illumos solaris

package router

import (
	"fmt"
	"log"
	"net/netip"
	"os/exec"

	"github.com/tailscale/wireguard-go/tun"
	"go4.org/netipx"
	"tailscale.com/health"
	"tailscale.com/net/netmon"
	"tailscale.com/types/logger"
)

type userspaceSunosRouter struct {
	logf    logger.Logf
	linkMon *netmon.Monitor
	health  *health.Tracker
	tunname string
	local   []netip.Prefix
	routes  map[netip.Prefix]struct{}
}

func newUserspaceSunosRouter(logf logger.Logf, tundev tun.Device, linkMon *netmon.Monitor, health *health.Tracker) (Router, error) {
	tunname, err := tundev.Name()
	if err != nil {
		return nil, err
	}

	return &userspaceSunosRouter{
		logf:    logf,
		linkMon: linkMon,
		health:  health,
		tunname: tunname,
	}, nil
}

func (r *userspaceSunosRouter) addrsToRemove(newLocalAddrs []netip.Prefix) (remove []netip.Prefix) {
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

func (r *userspaceSunosRouter) addrsToAdd(newLocalAddrs []netip.Prefix) (add []netip.Prefix) {
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

func cmdVerbose(logf logger.Logf, args []string) (string, error) {
	o, err := cmd(args...).CombinedOutput()
	out := string(o)
	if err != nil {
		logf("cmd %v failed: %v\n%s", args, err, string(out))
	}
	return out, err
}

func (r *userspaceSunosRouter) Up() error {
	ifup := []string{"ifconfig", r.tunname, "up"}
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		r.logf("running ifconfig failed: %v\n%s", err, out)
		// this seems to fail harmlessly on illumos
		//return err
	}
	return nil
}

func inet(p netip.Prefix) string {
	if p.Addr().Is6() {
		return "inet6"
	}
	return "inet"
}

func (r *userspaceSunosRouter) Set(cfg *Config) (reterr error) {
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
		if addr.Addr().Is4() && firstGateway4 == "" {
			firstGateway4 = addr.Addr().String()
		} else if addr.Addr().Is6() && firstGateway6 == "" {
			firstGateway6 = addr.Addr().String()
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
		addrString := fmt.Sprintf("local=%s,remote=%s", addr.String(), addr.Addr().String())
		addrObj := r.tunname + "/tailscale" + inet(addr)
		// TODO(2024-05-18) fix will be a year old. remove workaround
		// This is a mitigation to odd behaviour first noticed in 1.44, but that may have existed even before that...
		// It is *probably* https://www.illumos.org/issues/13316 based on the system where it was seen
		// I will leave this in place for a while until most distros have pulled in the fix which landed
		// in upstream illumos on Thu, 18 May 2023 01:24:32 +0000
		var arg0 = []string{"ipadm", "delete-addr", addrObj}
		_, err := cmd(arg0...).CombinedOutput()
		// Under normal circumstances this should fail. If it didn't we have tripped the bug and should log it.
		if err == nil {
			r.logf("BUG: unexpected delete-addr success for addrobj: %s", addrObj)
		}
		var arg = []string{"ipadm", "create-addr", "-t", "-T", "static", "-a", addrString, addrObj}
		out, err := cmd(arg...).CombinedOutput()
		if err != nil {
			r.logf("addr add failed: %v => %v\n%s", arg, err, out)
			setErr(err)
		}
		var arg2 = []string{"ifconfig"}
		out, err = cmd(arg2...).CombinedOutput()
		r.logf("%v => %v\n%s", arg, err, out)
	}

	newRoutes := make(map[netip.Prefix]struct{})
	for _, route := range cfg.Routes {
		newRoutes[route] = struct{}{}
	}
	// Delete any pre-existing routes.
	for route := range r.routes {
		if _, keep := newRoutes[route]; !keep {
			net := netipx.PrefixIPNet(route)
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			del := "delete"
			routedel := []string{"route", "-q", "-n",
				del, "-" + inet(route), nstr,
				"-iface", r.tunname}
			out, err := cmd(routedel...).CombinedOutput()
			if err != nil {
				r.logf("route delete failed: %v: %v\n%s", routedel, err, out)
				setErr(err)
			}
		}
	}
	for route := range newRoutes {
		if _, exists := r.routes[route]; !exists {
			net := netipx.PrefixIPNet(route)
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			var gateway string
			if route.Addr().Is4() && firstGateway4 != "" {
				gateway = firstGateway4
			}
			if route.Addr().Is6() && firstGateway6 != "" {
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
		r.local = append([]netip.Prefix{}, cfg.LocalAddrs...)
	}
	r.routes = newRoutes

	return errq
}

func (r *userspaceSunosRouter) Close() error {
	cleanUp(r.logf, r.tunname)
	return nil
}

// UpdateMagicsockPort implements the Router interface. This implementation
// does nothing and returns nil because this router does not currently need
// to know what the magicsock UDP port is.
func (r *userspaceSunosRouter) UpdateMagicsockPort(_ uint16, _ string) error {
	return nil
}
