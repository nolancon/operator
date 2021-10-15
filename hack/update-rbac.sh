#! /bin/bash -e

# This script fetches given project's manifests by version.
# Splits the multi-doc manifests into single-doc files.
# Updates given project version in channels/stable file.
# Replaces the given project manifests in operator repo with the new version.

: ${NAME?= required}
: ${VERSION?= required}
: ${FROM?= required}
: ${TO?= required}

if [[ $VERSION =~ ^v[0-9] ]]; then
    VERSION=${VERSION#"v"}
fi

WORKDIR=channels/packages/${NAME}/${VERSION}
CONFIGDIR=config/rbac/bases

sed '/rules:/Q' $CONFIGDIR/$TO > $CONFIGDIR/tmp_$TO
sed -n '/^rules:$/,$p' $WORKDIR/$FROM >> $CONFIGDIR/tmp_$TO

mv -f $CONFIGDIR/tmp_$TO $CONFIGDIR/$TO
