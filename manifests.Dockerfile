# Nixery image registry generates images on the fly with the counted dependencies.
FROM nixery.dev/shell/remake/go_1_16/gcc/kustomize as build

ENV PATH=/share/go/bin:$PATH

ARG OPERATOR_IMAGE=storageos/operator:develop

WORKDIR /tmp/src

COPY . .

RUN rm -rf /tmp/src/bin ; remake manifests
RUN kustomize build config/default > storageos-operator.yaml

# Create the final image.

FROM nixery.dev/shell

COPY --from=build /tmp/src/storageos-operator.yaml /operator.yaml

ENTRYPOINT cat /operator.yaml