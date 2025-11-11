#!/bin/bash

set -o xtrace
set -o errexit

export TS_USE_TOOLCHAIN=true

# feature tags to use in our version of the "box" build
BOX_TAGS="$(go run ./cmd/featuretags --min --add=osrouter,unixsocketidentity),ts_include_cli"

# This prevents illumos libc from leaking into Solaris binaries when built on illumos
export CGO_ENABLED=0

fix_osabi () {
	if [[ $(uname -s) == SunOS ]]; then
		/usr/bin/elfedit \
			-e "ehdr:ei_osabi ELFOSABI_SOLARIS" \
			-e "ehdr:ei_abiversion EAV_SUNW_CURRENT" \
			"${1?}"
	else
		elfedit --output-osabi "Solaris" --output-abiversion "1" "${1?}"
	fi
}

for GOOS in illumos solaris; do
	export GOOS
	TAGS=ts_include_cli bash -x ./build_dist.sh ./cmd/tailscaled
	fix_osabi tailscaled
	mv tailscaled{,-${GOOS}}
	TAGS=${BOX_TAGS} bash -x ./build_dist.sh ./cmd/tailscaled
	fix_osabi tailscaled
	mv tailscaled{,-minimal-${GOOS}}
	# Build plain daemon binary
	bash -x ./build_dist.sh ./cmd/tailscaled
	fix_osabi tailscaled
	mv tailscaled{,-plain-${GOOS}}
	# Build plain client binary
	bash -x ./build_dist.sh ./cmd/tailscale
	fix_osabi tailscale
	mv tailscale{,-${GOOS}}
done

ln cmd/tailscaled/smf/tailscale.xml .
ln cmd/tailscaled/smf/vpn-tailscale .
ln cmd/tailscaled/smf/install .
shasum -a 256 install tailscaled-* tailscale-* tailscale.xml vpn-tailscale>sha256sums
rm ./tailscale.xml ./vpn-tailscale ./install
