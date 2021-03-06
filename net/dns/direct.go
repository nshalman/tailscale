// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux freebsd openbsd

package dns

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"inet.af/netaddr"
	"tailscale.com/atomicfile"
	"tailscale.com/util/dnsname"
)

const (
	backupConf = "/etc/resolv.pre-tailscale-backup.conf"
	resolvConf = "/etc/resolv.conf"
)

// writeResolvConf writes DNS configuration in resolv.conf format to the given writer.
func writeResolvConf(w io.Writer, servers []netaddr.IP, domains []dnsname.FQDN) {
	io.WriteString(w, "# resolv.conf(5) file generated by tailscale\n")
	io.WriteString(w, "# DO NOT EDIT THIS FILE BY HAND -- CHANGES WILL BE OVERWRITTEN\n\n")
	for _, ns := range servers {
		io.WriteString(w, "nameserver ")
		io.WriteString(w, ns.String())
		io.WriteString(w, "\n")
	}
	if len(domains) > 0 {
		io.WriteString(w, "search")
		for _, domain := range domains {
			io.WriteString(w, " ")
			io.WriteString(w, domain.WithoutTrailingDot())
		}
		io.WriteString(w, "\n")
	}
}

func readResolv(r io.Reader) (config OSConfig, err error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "nameserver") {
			nameserver := strings.TrimPrefix(line, "nameserver")
			nameserver = strings.TrimSpace(nameserver)
			ip, err := netaddr.ParseIP(nameserver)
			if err != nil {
				return OSConfig{}, err
			}
			config.Nameservers = append(config.Nameservers, ip)
			continue
		}

		if strings.HasPrefix(line, "search") {
			domain := strings.TrimPrefix(line, "search")
			domain = strings.TrimSpace(domain)
			fqdn, err := dnsname.ToFQDN(domain)
			if err != nil {
				return OSConfig{}, fmt.Errorf("parsing search domains %q: %w", line, err)
			}
			config.SearchDomains = append(config.SearchDomains, fqdn)
			continue
		}
	}

	return config, nil
}

func readResolvFile(path string) (OSConfig, error) {
	var config OSConfig

	f, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer f.Close()

	return readResolv(f)
}

// readResolvConf reads DNS configuration from /etc/resolv.conf.
func readResolvConf() (OSConfig, error) {
	return readResolvFile(resolvConf)
}

// resolvOwner returns the apparent owner of the resolv.conf
// configuration in bs - one of "resolvconf", "systemd-resolved" or
// "NetworkManager", or "" if no known owner was found.
func resolvOwner(bs []byte) string {
	b := bytes.NewBuffer(bs)
	for {
		line, err := b.ReadString('\n')
		if err != nil {
			return ""
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line[0] != '#' {
			// First non-empty, non-comment line. Assume the owner
			// isn't hiding further down.
			return ""
		}

		if strings.Contains(line, "systemd-resolved") {
			return "systemd-resolved"
		} else if strings.Contains(line, "NetworkManager") {
			return "NetworkManager"
		} else if strings.Contains(line, "resolvconf") {
			return "resolvconf"
		}
	}
}

// isResolvedRunning reports whether systemd-resolved is running on the system,
// even if it is not managing the system DNS settings.
func isResolvedRunning() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	// systemd-resolved is never installed without systemd.
	_, err := exec.LookPath("systemctl")
	if err != nil {
		return false
	}

	// is-active exits with code 3 if the service is not active.
	err = exec.Command("systemctl", "is-active", "systemd-resolved.service").Run()

	return err == nil
}

// directManager is a managerImpl which replaces /etc/resolv.conf with a file
// generated from the given configuration, creating a backup of its old state.
//
// This way of configuring DNS is precarious, since it does not react
// to the disappearance of the Tailscale interface.
// The caller must call Down before program shutdown
// or as cleanup if the program terminates unexpectedly.
type directManager struct{}

func newDirectManager() (directManager, error) {
	return directManager{}, nil
}

// ownedByTailscale reports whether /etc/resolv.conf seems to be a
// tailscale-managed file.
func (m directManager) ownedByTailscale() (bool, error) {
	st, err := os.Stat(resolvConf)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !st.Mode().IsRegular() {
		return false, nil
	}
	bs, err := ioutil.ReadFile(resolvConf)
	if err != nil {
		return false, err
	}
	if bytes.Contains(bs, []byte("generated by tailscale")) {
		return true, nil
	}
	return false, nil
}

// backupConfig creates or updates a backup of /etc/resolv.conf, if
// resolv.conf does not currently contain a Tailscale-managed config.
func (m directManager) backupConfig() error {
	if _, err := os.Stat(resolvConf); err != nil {
		if os.IsNotExist(err) {
			// No resolv.conf, nothing to back up. Also get rid of any
			// existing backup file, to avoid restoring something old.
			os.Remove(backupConf)
			return nil
		}
		return err
	}

	owned, err := m.ownedByTailscale()
	if err != nil {
		return err
	}
	if owned {
		return nil
	}

	return os.Rename(resolvConf, backupConf)
}

func (m directManager) restoreBackup() error {
	if _, err := os.Stat(backupConf); err != nil {
		if os.IsNotExist(err) {
			// No backup, nothing we can do.
			return nil
		}
		return err
	}
	owned, err := m.ownedByTailscale()
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolvConf); err != nil && !os.IsNotExist(err) {
		return err
	}
	resolvConfExists := !os.IsNotExist(err)

	if resolvConfExists && !owned {
		// There's already a non-tailscale config in place, get rid of
		// our backup.
		os.Remove(backupConf)
		return nil
	}

	// We own resolv.conf, and a backup exists.
	if err := os.Rename(backupConf, resolvConf); err != nil {
		return err
	}

	return nil
}

func (m directManager) SetDNS(config OSConfig) error {
	if config.IsZero() {
		if err := m.restoreBackup(); err != nil {
			return err
		}
	} else {
		if err := m.backupConfig(); err != nil {
			return err
		}

		buf := new(bytes.Buffer)
		writeResolvConf(buf, config.Nameservers, config.SearchDomains)
		if err := atomicfile.WriteFile(resolvConf, buf.Bytes(), 0644); err != nil {
			return err
		}
	}

	// We might have taken over a configuration managed by resolved,
	// in which case it will notice this on restart and gracefully
	// start using our configuration. This shouldn't happen because we
	// try to manage DNS through resolved when it's around, but as a
	// best-effort fallback if we messed up the detection, try to
	// restart resolved to make the system configuration consistent.
	if isResolvedRunning() {
		exec.Command("systemctl", "restart", "systemd-resolved.service").Run()
	}

	return nil
}

func (m directManager) SupportsSplitDNS() bool {
	return false
}

func (m directManager) GetBaseConfig() (OSConfig, error) {
	owned, err := m.ownedByTailscale()
	if err != nil {
		return OSConfig{}, err
	}
	fileToRead := resolvConf
	if owned {
		fileToRead = backupConf
	}

	return readResolvFile(fileToRead)
}

func (m directManager) Close() error {
	// We used to keep a file for the tailscale config and symlinked
	// to it, but then we stopped because /etc/resolv.conf being a
	// symlink to surprising places breaks snaps and other sandboxing
	// things. Clean it up if it's still there.
	os.Remove("/etc/resolv.tailscale.conf")

	if _, err := os.Stat(backupConf); err != nil {
		if os.IsNotExist(err) {
			// No backup, nothing we can do.
			return nil
		}
		return err
	}
	owned, err := m.ownedByTailscale()
	if err != nil {
		return err
	}
	_, err = os.Stat(resolvConf)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	resolvConfExists := !os.IsNotExist(err)

	if resolvConfExists && !owned {
		// There's already a non-tailscale config in place, get rid of
		// our backup.
		os.Remove(backupConf)
		return nil
	}

	// We own resolv.conf, and a backup exists.
	if err := os.Rename(backupConf, resolvConf); err != nil {
		return err
	}

	if isResolvedRunning() {
		exec.Command("systemctl", "restart", "systemd-resolved.service").Run() // Best-effort.
	}

	return nil
}
