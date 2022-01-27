#! /bin/bash -e

# This script fetches given project's manifests by version.
# Splits the multi-doc manifests into single-doc files.
# Updates given project version in channels/stable file.
# Replaces the given project manifests in operator repo with the new version.

# shellcheck disable=SC2059

: "${NAME?= required}"
: "${VERSION?= required}"
: "${MANIFESTS_IMAGE?= required}"

if [[ "$VERSION" =~ ^v[0-9] ]]; then
    VERSION=${VERSION#"v"}
fi

WORKDIR=channels/packages/${NAME}/${VERSION}

saveManifest() {
    : "${1?= manifest required}"

    kind=$(echo "$1" | grep -E "^kind:" | head -1 | cut -d":" -f2- | tr -d ' ')
    name=$(echo "$1" | grep -E "^  name:" | head -1 | cut -d":" -f2- | tr -d ' ')

    echo "$1" > "$WORKDIR/${kind,,}-${name//:/-}.yaml"
    echo "- ${kind,,}-${name//:/-}.yaml" >> "${WORKDIR}"/kustomization.yaml
}

rm -rf "${WORKDIR}"
mkdir -p "${WORKDIR}"

echo "resources:" > "${WORKDIR}"/kustomization.yaml

MANIFESTS=$(docker run --rm "${MANIFESTS_IMAGE}")

while IFS= read -r line; do
    if [[ $line == "---" ]]; then
        saveManifest "$MANIFEST"
        MANIFEST=""
        continue
    fi
    MANIFEST="$(printf "${MANIFEST}\n${line}")"
done <<< "$MANIFESTS"

saveManifest "$MANIFEST"

sed -i "/- name: ${NAME}/!b;n;c\  version: ${VERSION}" channels/stable
