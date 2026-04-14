package client

import (
	"net"
	"net/url"
	"os"

	prismgoclient "github.com/nutanix-cloud-native/prism-go-client"
	v4Converged "github.com/nutanix-cloud-native/prism-go-client/converged/v4"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
	"k8s.io/klog/v2"
)

const (
	ProviderName = "nutanix"

	// GlobalInfrastuctureName default name for infrastructure object
	GlobalInfrastuctureName = "cluster"

	// Nutanix credential keys
	NutanixEndpointKey = "NUTANIX_PRISM_CENTRAL_ENDPOINT"
	NutanixPortKey     = "NUTANIX_PRISM_CENTRAL_PORT"
	NutanixUserKey     = "NUTANIX_PRISM_CENTRAL_USER"
	NutanixPasswordKey = "NUTANIX_PRISM_CENTRAL_PASSWORD"
)

type ClientOptions struct {
	Credentials *prismgoclient.Credentials
}

// resolveCredentials populates credentials from environment variables if not already set,
// and builds the URL if missing.
func resolveCredentials(options *ClientOptions) {
	if options.Credentials == nil {
		options.Credentials = &prismgoclient.Credentials{
			Username: getEnvVar(NutanixUserKey),
			Password: getEnvVar(NutanixPasswordKey),
			Port:     getEnvVar(NutanixPortKey),
			Endpoint: getEnvVar(NutanixEndpointKey),
		}
	}
	if len(options.Credentials.URL) == 0 {
		options.Credentials.URL = (&url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(options.Credentials.Endpoint, options.Credentials.Port),
		}).String()
	}
}

// Client returns a converged v4 Nutanix client.
func Client(options *ClientOptions) (*v4Converged.Client, error) {
	resolveCredentials(options)

	klog.Infof("Creating nutanix converged client with creds: (url: %s, insecure: %v)",
		options.Credentials.URL, options.Credentials.Insecure)
	cli, err := v4Converged.NewClient(*options.Credentials)
	if err != nil {
		klog.Errorf("Failed to create the nutanix converged client: %v", err)
		return nil, err
	}

	return cli, nil
}

// V3Client returns a v3 Nutanix client for APIs not yet available in the converged v4 client (e.g. Projects).
func V3Client(options *ClientOptions) (*nutanixClientV3.Client, error) {
	resolveCredentials(options)

	klog.Infof("Creating nutanix v3 client for project APIs with creds: (url: %s, insecure: %v)",
		options.Credentials.URL, options.Credentials.Insecure)
	cli, err := nutanixClientV3.NewV3Client(*options.Credentials)
	if err != nil {
		klog.Errorf("Failed to create the nutanix v3 client: %v", err)
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
