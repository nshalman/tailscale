// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsdns

import (
	"github.com/miekg/dns"
	"inet.af/netaddr"
)

// This file exists to isolate the test infrastructure
// that depends on github.com/miekg/dns
// from the rest, which only depends on dnsmessage.

var dnsHandleFunc = dns.HandleFunc

// resolveToIP returns a handler function which responds
// to queries of type A it receives with an A record containing ipv4,
// to queries of type AAAA with an AAAA record containing ipv6,
// to queries of type NS with an NS record containg name.
func resolveToIP(ipv4, ipv6 netaddr.IP, ns string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)

		if len(req.Question) != 1 {
			panic("not a single-question request")
		}
		question := req.Question[0]

		var ans dns.RR
		switch question.Qtype {
		case dns.TypeA:
			ans = &dns.A{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: ipv4.IPAddr().IP,
			}
		case dns.TypeAAAA:
			ans = &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
				},
				AAAA: ipv6.IPAddr().IP,
			}
		case dns.TypeNS:
			ans = &dns.NS{
				Hdr: dns.RR_Header{
					Name:   question.Name,
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: ns,
			}
		}

		m.Answer = append(m.Answer, ans)
		w.WriteMsg(m)
	}
}

func resolveToNXDOMAIN(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetRcode(req, dns.RcodeNameError)
	w.WriteMsg(m)
}

func serveDNS(addr string) (*dns.Server, chan error) {
	server := &dns.Server{Addr: addr, Net: "udp"}

	waitch := make(chan struct{})
	server.NotifyStartedFunc = func() { close(waitch) }

	errch := make(chan error, 1)
	go func() {
		errch <- server.ListenAndServe()
		close(errch)
	}()

	<-waitch
	return server, errch
}
