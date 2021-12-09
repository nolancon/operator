#!/bin/bash

: ${NAME?= required}
: ${NAMESPACE?= required}

kubectl get storageoscluster -n $NAMESPACE $NAME -o yaml | sed -e 's|^spec:$|spec:\n  nodeManagerFeatures:\n    foo: bar|' | kubectl apply -f -
