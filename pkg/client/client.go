package client

import (
	"fmt"
	"os"

	nutanixClient "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/pkg/nutanix/v3"
	"k8s.io/klog/v2"
)

const (
	ProviderName = "nutanix"

	// GlobalInfrastuctureName default name for infrastructure object
	GlobalInfrastuctureName = "cluster"

	// KubeCloudConfigNamespace is the namespace where the kube cloud config ConfigMap is located
	KubeCloudConfigNamespace = "openshift-config-managed"
	// kubeCloudConfigName is the name of the kube cloud config ConfigMap
	kubeCloudConfigName = "kube-cloud-config"
	// cloudCABundleKey is the key in the kube cloud config ConfigMap where the custom CA bundle is located
	cloudCABundleKey = "ca-bundle.pem"

	// Nutanix credential keys
	NutanixEndpointKey = "Nutanix_PrismCentral_Endpoint"
	NutanixPortKey     = "Nutanix_PrismCentral_Port"
	NutanixUserKey     = "Nutanix_PrismCentral_User"
	NutanixPasswordKey = "Nutanix_PrismCentral_Password"
)

type ClientOptions struct {
	Credentials *nutanixClient.Credentials
	Debug       bool
}

func Client(options *ClientOptions) (*nutanixClientV3.Client, error) {
	if options.Credentials == nil {
		username := getEnvVar(NutanixUserKey)
		password := getEnvVar(NutanixPasswordKey)
		port := getEnvVar(NutanixPortKey)
		endpoint := getEnvVar(NutanixEndpointKey)
		options.Credentials = &nutanixClient.Credentials{
			Username: username,
			Password: password,
			Port:     port,
			Endpoint: endpoint,
		}
	}

	if len(options.Credentials.URL) == 0 {
		options.Credentials.URL = fmt.Sprintf("%s:%s", options.Credentials.Endpoint, options.Credentials.Port)
	}

	// FIXME: Use insure connection for now. Will change to use secure connection later
	options.Credentials.Insecure = true

	klog.Infof("To create nutanixClient with creds: (url: %s, insecure: %v)", options.Credentials.URL, options.Credentials.Insecure)
	cli, err := nutanixClientV3.NewV3Client(*options.Credentials, options.Debug)
	if err != nil {
		klog.Errorf("Failed to create the nutanix client. error: %v", err)
		return nil, err
	}

	return cli, nil
}

func getEnvVar(key string) (val string) {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return
}
