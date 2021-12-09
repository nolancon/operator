#!/bin/bash

RESTARTS=$(kubectl get po -n storageos -l app.kubernetes.io/component=operator -o custom-columns=:.status.containerStatuses[1].restartCount --no-headers)
kubectl get cm -n storageos storageos-related-images -o yaml | sed -e 's|^data:$|data:\n  foo: bar|' | kubectl apply -f -
while [[ "$RESTARTS" == "$(kubectl get po -n storageos -l app.kubernetes.io/component=operator -o custom-columns=:.status.containerStatuses[1].restartCount --no-headers)" ]]; do
    sleep 5
done