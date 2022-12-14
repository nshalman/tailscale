#!/bin/bash

set -o xtrace
set -o errexit


export GOOS=${1-illumos}

# This prevents illumos libc from leaking into Solaris binaries when built on illumos
export CGO_ENABLED=0

for cmd in ./cmd/tailscale ./cmd/tailscaled; do
	bash -x ./build_dist.sh "${cmd}"
	if [[ $(uname -s) == SunOS ]]; then
		/usr/bin/elfedit \
			-e "ehdr:ei_osabi ELFOSABI_SOLARIS" \
			-e "ehdr:ei_abiversion EAV_SUNW_CURRENT" \
			"$(basename $cmd)"
	else
		elfedit --output-osabi "Solaris" --output-abiversion "1" "$(basename $cmd)"
	fi
done

pkgver=$(cat VERSION.txt)
pkgdir=${pkgver}-${GOOS}

mkdir ${pkgdir}
mv tailscale tailscaled ${pkgdir}
cp cmd/tailscaled/tailscale.xml ${pkgdir}
cp $0 ${pkgdir}/build.sh
cd ${pkgdir}
shasum -a 256 * >sha256sums
cat >index.html <<EOF
<html>
<head><title>${GOOS} build of Tailscale ${pkgver}</title></head>
${GOOS} build of Tailscale ${pkgver}
<ul>
<li><a href="tailscale">tailscale</a></li>
<li><a href="tailscaled">tailscaled</a></li>
<li><a href="tailscale.xml">tailscale.xml</a></li>
<li><a href="sha256sums">sha256sums</a></li>
<li><a href="build.sh">build.sh</a></li>
</ul>
</html>
EOF
