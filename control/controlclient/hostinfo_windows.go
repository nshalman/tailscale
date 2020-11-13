// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package controlclient

import (
	"os/exec"
	"strings"
	"syscall"
)

func init() {
	osVersion = osVersionWindows
}

func osVersionWindows() string {
	cmd := exec.Command("cmd", "/c", "ver")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, _ := cmd.Output() // "\nMicrosoft Windows [Version 10.0.19041.388]\n\n"
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "Microsoft Windows [")
	s = strings.TrimSuffix(s, "]")

	// "Version 10.x.y.z", with "Version" localized. Keep only stuff after the space.
	if sp := strings.Index(s, " "); sp != -1 {
		s = s[sp+1:]
	}
	return s // "10.0.19041.388", ideally
}
