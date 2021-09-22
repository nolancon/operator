#! /bin/bash -e

# This script fetches api-manager's manifests by version.
# Splits the multi-doc manifests into single-doc files.
# Updates api-manager version in channels/stable file.
# Replaces the api-manager manifests in operator repo with the new version.

: ${API_MANAGER_VERSION?= required}
: ${API_MANAGER_MANIFESTS_IMAGE?= required}

if [[ $API_MANAGER_VERSION =~ ^v[0-9] ]]; then
    API_MANAGER_VERSION=${API_MANAGER_VERSION#"v"}
fi

WORKDIR=channels/packages/api-manager/${API_MANAGER_VERSION}

saveManifest() {
    : ${1?= manifest required}

    kind=$(echo "$1" | egrep "^kind:" | head -1 | cut -d":" -f2- | tr -d ' ')
    name=$(echo "$1" | egrep "^  name:" | head -1 | cut -d":" -f2- | tr -d ' ')

    echo "$1" > "$WORKDIR/${kind,,}-${name//:/-}.yaml"
    echo "- ${kind,,}-${name//:/-}.yaml" >> ${WORKDIR}/kustomization.yaml
}

rm -rf ${WORKDIR}
mkdir -p ${WORKDIR}

echo "resources:" > ${WORKDIR}/kustomization.yaml

MANIFESTS=$(docker run --rm ${API_MANAGER_MANIFESTS_IMAGE})

while IFS= read -r line; do
    if [[ $line == "---" ]]; then
        saveManifest "$MANIFEST"
        MANIFEST=""
        continue
    fi
    MANIFEST=$(printf "${MANIFEST}\n${line}")
done <<< "$MANIFESTS"

saveManifest "$MANIFEST"

sed -i "/- name: api-manager/!b;n;c\  version: ${API_MANAGER_VERSION}" channels/stable
