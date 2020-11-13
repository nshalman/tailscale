// Copyright (c) 2020 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dns

import (
	"time"

	"tailscale.com/types/logger"
)

// We use file-ignore below instead of ignore because on some platforms,
// the lint exception is necessary and on others it is not,
// and plain ignore complains if the exception is unnecessary.

//lint:file-ignore U1000 reconfigTimeout is used on some platforms but not others

// reconfigTimeout is the time interval within which Manager.{Up,Down} should complete.
//
// This is particularly useful because certain conditions can cause indefinite hangs
// (such as improper dbus auth followed by contextless dbus.Object.Call).
// Such operations should be wrapped in a timeout context.
const reconfigTimeout = time.Second

type managerImpl interface {
	// Up updates system DNS settings to match the given configuration.
	Up(Config) error
	// Down undoes the effects of Up.
	// It is idempotent and performs no action if Up has never been called.
	Down() error
}

// Manager manages system DNS settings.
type Manager struct {
	logf logger.Logf

	impl managerImpl

	config  Config
	mconfig ManagerConfig
}

// NewManagers created a new manager from the given config.
func NewManager(mconfig ManagerConfig) *Manager {
	mconfig.Logf = logger.WithPrefix(mconfig.Logf, "dns: ")
	m := &Manager{
		logf: mconfig.Logf,
		impl: newManager(mconfig),

		config:  Config{PerDomain: mconfig.PerDomain},
		mconfig: mconfig,
	}

	m.logf("using %T", m.impl)
	return m
}

func (m *Manager) Set(config Config) error {
	if config.Equal(m.config) {
		return nil
	}

	m.logf("Set: %+v", config)

	if len(config.Nameservers) == 0 {
		err := m.impl.Down()
		// If we save the config, we will not retry next time. Only do this on success.
		if err == nil {
			m.config = config
		}
		return err
	}

	// Switching to and from per-domain mode may require a change of manager.
	if config.PerDomain != m.config.PerDomain {
		if err := m.impl.Down(); err != nil {
			return err
		}
		m.mconfig.PerDomain = config.PerDomain
		m.impl = newManager(m.mconfig)
		m.logf("switched to %T", m.impl)
	}

	err := m.impl.Up(config)
	// If we save the config, we will not retry next time. Only do this on success.
	if err == nil {
		m.config = config
	}

	return err
}

func (m *Manager) Up() error {
	return m.impl.Up(m.config)
}

func (m *Manager) Down() error {
	return m.impl.Down()
}
