#!/bin/bash

: ${NAME?= required}
: ${NAMESPACE?= required}

kubectl get storageoscluster -n $NAMESPACE $NAME -o yaml | sed -e 's|^spec:$|spec:\n  nodeManagerFeatures:\n    upgradeGuard: ""|' | kubectl apply -f -
