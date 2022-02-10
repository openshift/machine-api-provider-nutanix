FROM registry.ci.openshift.org/openshift/release:golang-1.17 AS builder
WORKDIR /go/src/github.com/nutanix-cloud-native/machine-api-provider-nutanix
COPY . .
# VERSION env gets set in the openshift/release image and refers to the golang version, which interfers with our own
#RUN make build
RUN unset VERSION && GOPROXY=off NO_DOCKER=1 make build

FROM registry.ci.openshift.org/origin/4.9:base
COPY --from=builder /go/src/github.com/nutanix-cloud-native/machine-api-provider-nutanix/bin/machine-controller-manager /
