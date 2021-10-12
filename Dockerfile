# Use ubi8-minimal as the base image to package the manager binary. Refer to
# https://catalog.redhat.com/software/containers/ubi8/ubi-minimal/5c359a62bed8bd75a2c3fba8
# for more details
FROM registry.access.redhat.com/ubi8/ubi-minimal
ARG VERSION=v1.0.0
WORKDIR /
LABEL name="StorageOS Operator" \
    maintainer="support@storageos.com" \
    vendor="StorageOS" \
    version="${VERSION}" \
    release="2" \
    distribution-scope="public" \
    architecture="x86_64" \
    url="https://docs.storageos.com" \
    io.k8s.description="The StorageOS Operator installs and manages StorageOS within a cluster." \
    io.k8s.display-name="StorageOS Operator" \
    io.openshift.tags="" \
    summary="Highly-available persistent block storage for containerized applications." \
    description="StorageOS transforms commodity server or cloud based disk capacity into enterprise-class storage to run persistent workloads such as databases in containers. Provides high availability, low latency persistent block storage. No other hardware or software is required."
RUN mkdir -p /licenses
COPY LICENSE /licenses/LICENSE
COPY channels channels
COPY manager manager
USER 65532:65532

ENTRYPOINT ["/manager"]
