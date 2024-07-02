package machine

import (
	"github.com/nutanix-cloud-native/prism-go-client/utils"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultCredentialsSecretName is the default name of the secret holding the credentials for PC client
	defaultCredentialsSecretName = "nutanix-credentials"

	userDataSecretName = "nutanix-userdata"

	credentialsData = `[{"type":"basic_auth","data":{"prismCentral":{"username":"pc_user","password":"pc_password"},"prismElements":[{"name":"pe_name","username":"pe_user","password":"pe_password"}]}}]`
)

var (
	infrastructure = configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: globalInfrastuctureName,
		},
		Spec: configv1.InfrastructureSpec{
			CloudConfig: configv1.ConfigMapFileReference{},
			PlatformSpec: configv1.PlatformSpec{
				Type: configv1.NutanixPlatformType,
				Nutanix: &configv1.NutanixPlatformSpec{
					PrismCentral: configv1.NutanixPrismEndpoint{Address: "10.40.42.15", Port: 9440},
					PrismElements: []configv1.NutanixPrismElementEndpoint{
						{Name: "test-pe", Endpoint: configv1.NutanixPrismEndpoint{Address: "10.40.131.30", Port: 9440}},
					},
				},
			},
		},
	}

	infrastructureStatus = configv1.InfrastructureStatus{
		InfrastructureName:     "test-cluster-1",
		ControlPlaneTopology:   configv1.HighlyAvailableTopologyMode,
		InfrastructureTopology: configv1.SingleReplicaTopologyMode,
		PlatformStatus: &configv1.PlatformStatus{
			Type: configv1.NutanixPlatformType,
			Nutanix: &configv1.NutanixPlatformStatus{
				APIServerInternalIPs: []string{"10.40.42.5"},
				IngressIPs:           []string{"10.40.42.6"},
			},
		},
	}

	failureDomains = []configv1.NutanixFailureDomain{
		{
			Name: "fd.pe0",
			Cluster: configv1.NutanixResourceIdentifier{
				Type: configv1.NutanixIdentifierName,
				Name: Ptr[string]("pe0"),
			},
			Subnets: []configv1.NutanixResourceIdentifier{{
				Type: configv1.NutanixIdentifierName,
				Name: Ptr[string]("pe0-net"),
			}},
		},
		{
			Name: "fd.pe1",
			Cluster: configv1.NutanixResourceIdentifier{
				Type: configv1.NutanixIdentifierUUID,
				UUID: Ptr[string]("0005a0f3-8f43-a0f5-02b7-3cecef194315"),
			},
			Subnets: []configv1.NutanixResourceIdentifier{{
				Type: configv1.NutanixIdentifierName,
				Name: Ptr[string]("pe1-net"),
			}},
		},
		{
			Name: "fd.pe2",
			Cluster: configv1.NutanixResourceIdentifier{
				Type: configv1.NutanixIdentifierName,
				Name: Ptr[string]("pe2"),
			},
			Subnets: []configv1.NutanixResourceIdentifier{{
				Type: configv1.NutanixIdentifierUUID,
				UUID: Ptr[string]("a8938dc6-7659-6801-a688-e26020c68241"),
			}},
		},
	}
)

// getInfrastructureObject returns the Infrastructure object for test
func getInfrastructureObject(withFailureDomains bool) *configv1.Infrastructure {
	infra := infrastructure.DeepCopy()

	if withFailureDomains {
		fds := []configv1.NutanixFailureDomain{}
		for _, fd := range failureDomains {
			fds = append(fds, fd)
		}
		infra.Spec.PlatformSpec.Nutanix.FailureDomains = fds
	}

	return infra
}

// getValidProviderSpec returns the valid NutanixMachineProviderConfig object for test
func getValidProviderSpec(fdName string) *machinev1.NutanixMachineProviderConfig {

	return &machinev1.NutanixMachineProviderConfig{
		Cluster: machinev1.NutanixResourceIdentifier{Type: "name", Name: utils.StringPtr("test-pe")},
		Image:   machinev1.NutanixResourceIdentifier{Type: "name", Name: utils.StringPtr("rhcos-4.10-nutanix")},
		Subnets: []machinev1.NutanixResourceIdentifier{
			{Type: "name", Name: Ptr[string]("test_net")},
		},
		VCPUsPerSocket: 2,
		VCPUSockets:    1,
		MemorySize:     resource.MustParse("4096Mi"),
		SystemDiskSize: resource.MustParse("120Gi"),
		CredentialsSecret: &corev1.LocalObjectReference{
			Name: defaultCredentialsSecretName,
		},
		UserDataSecret: &corev1.LocalObjectReference{
			Name: userDataSecretName,
		},
		FailureDomain: &machinev1.NutanixFailureDomainReference{
			Name: fdName,
		},
	}
}
