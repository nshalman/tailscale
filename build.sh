#!/bin/bash

set -o xtrace
set -o errexit

export TS_USE_TOOLCHAIN=true
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
	bash -x ./build_dist.sh --box ./cmd/tailscaled
	fix_osabi tailscaled
	mv tailscaled{,-${GOOS}}
done

ln cmd/tailscaled/tailscale.xml .
shasum -a 256 tailscaled-* tailscale.xml >sha256sums
rm ./tailscale.xml
