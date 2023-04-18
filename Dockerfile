FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.20-openshift-4.14 AS builder
WORKDIR /go/src/github.com/openshift/machine-api-provider-nutanix
COPY . .
# VERSION env gets set in the openshift/release image and refers to the golang version, which interfers with our own
RUN unset VERSION && GOPROXY=off NO_DOCKER=1 make build

FROM registry.ci.openshift.org/ocp/4.14:base
COPY --from=builder /go/src/github.com/openshift/machine-api-provider-nutanix/bin/machine-controller-manager /
