#!/bin/bash

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
KUBE_VERSION=$(kubectl version --short | grep Server | awk '{print $3}' | cut -d"." -f1-2)

case $KUBE_VERSION in
  v1.21 | v1.22 | v1.23 | v1.24 | v1.25 | v1.26)
    echo "set v1 for $KUBE_VERSION"
    sed -i 's|policy/v1beta1|policy/v1|' $SCRIPT_DIR/../tests/e2e/deployment-test/07-assert.yaml
    ;;

  *)
    echo "v1beta1 is ok for $KUBE_VERSION"
    ;;
esac
