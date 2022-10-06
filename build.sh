#!/bin/bash

set -o xtrace
set -o errexit

pkgver=$(git describe --tags --dirty)
_commit=$(git rev-parse HEAD)

export GOOS=${1-illumos}
GO_LDFLAGS="\
        -X tailscale.com/version.Long=${pkgver} \
        -X tailscale.com/version.Short=${pkgver} \
        -X tailscale.com/version.GitCommit=${_commit}"


for cmd in ./cmd/tailscale ./cmd/tailscaled; do
	go build -v -tags xversion -ldflags "$GO_LDFLAGS" "$cmd"
	# On SmartOS use platform elfedit, not pkgsrc one
	[[ ${GOOS} == illumos ]] && [[ $(uname -s) == SunOS ]] && /usr/bin/elfedit -e "ehdr:ei_osabi ELFOSABI_SOLARIS" "$(basename $cmd)"
done

mkdir ${pkgver}
mv tailscale tailscaled ${pkgver}
cp cmd/tailscaled/tailscale.xml ${pkgver}
cp $0 ${pkgver}/build.sh
cd ${pkgver}
shasum -a 256 * > sha256sums
cat >index.html <<EOF
<html>
<head><title>${GOOS} build of Tailscale ${pkgver}</title></head>
illumos build of Tailscale ${pkgver}
<ul>
<li><a href="tailscale">tailscale</a></li>
<li><a href="tailscaled">tailscaled</a></li>
<li><a href="tailscale.xml">tailscale.xml</a></li>
<li><a href="sha256sums">sha256sums</a></li>
<li><a href="build.sh">build.sh</a></li>
</ul>
</html>
EOF
