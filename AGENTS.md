This file provides guidance to agents when working with code in this repository.

This is the Nutanix Machine API Provider for OpenShift - a Kubernetes controller that provisions and manages virtual machines on Nutanix infrastructure. It contains:

- Machine controller that implements the OpenShift Machine API for Nutanix
- MachineSet validation controller for Nutanix-specific configurations
- Nutanix Prism Central API client wrapper
- Integration with OpenShift's machine-api-operator

## Key Architecture Components

### Machine Actuator (`pkg/actuators/machine/`)
The core controller implementing machine lifecycle operations (Create, Update, Delete, Exists) for Nutanix VMs.
- **actuator.go**: Main actuator interface implementation
- **reconciler.go**: Reconciliation logic and VM state management
- **machine_scope.go**: Encapsulates machine state and Nutanix provider configuration
- **vm.go**: Nutanix VM operations via Prism Central API
- **utils.go**: Helper functions for validation and condition management

### MachineSet Controller (`pkg/actuators/machineset/`)
Validates MachineSet configurations before machines are created:
- Provider spec validation
- Failure domain enforcement
- Nutanix-specific configuration checks

### Nutanix Client (`pkg/client/`)
Wrapper around nutanix-cloud-native/prism-go-client:
- Client initialization with credentials from environment
- State management and caching
- Authentication with Prism Central API

### Key Data Structures
- **NutanixMachineProviderConfig**: Machine configuration (cluster UUID, image, subnets, VM sizing, boot type, categories)
- **NutanixMachineProviderStatus**: Machine state tracking (VM UUID, addresses, conditions)

## Common Development Commands

### Building
```bash
make build              # Build machine-controller-manager binary
make images             # Build container image
make vendor             # Update vendored dependencies
make generate           # Run code generation
```

### Testing
```bash
make unit               # Run unit tests with Ginkgo/Gomega
make test-e2e           # Run end-to-end tests (requires Nutanix cluster)
make check              # Run all checks: fmt, vet, lint, test
```

### Code Quality
```bash
make fmt                # Format code with gofmt
make goimports          # Organize imports
make vet                # Run go vet
make lint               # Run golint
```


## Common Development Tasks

### Adding New Machine Configuration Fields

When adding new fields to NutanixMachineProviderConfig:

1. Update the API type definition with proper JSON tags and documentation
2. Add validation logic in `pkg/actuators/machineset/controller.go`
3. Implement the field handling in `pkg/actuators/machine/vm.go`
4. Update `machine_scope.go` if needed for accessing the new field
5. Add unit tests in corresponding `*_test.go` files
6. Run `make generate` to update generated code
7. Run `make check` to ensure code quality

Example: Adding VM category support would require changes to:
- API types (NutanixMachineProviderConfig)
- VM creation logic (vm.go)
- Validation (machineset controller)
- Unit tests

### Fixing Machine Creation/Deletion Issues

1. Check machine status conditions: `oc get machine <machine-name> -n <namespace> -o yaml`
2. Review controller logs: `oc logs -n <controller-namespace> deployment/machine-api-controllers -c machine-controller`
3. Verify Nutanix credentials and connectivity
4. Debug in `pkg/actuators/machine/reconciler.go` - main reconciliation logic
5. Check VM operations in `pkg/actuators/machine/vm.go`

Note: Default namespace is typically `openshift-machine-api` in production OpenShift clusters.

### Adding Nutanix API Features

When integrating new Nutanix Prism Central API features:

1. Check if prism-go-client supports the feature (update vendored dependency if needed)
2. Add client methods in `pkg/client/client.go` if necessary
3. Implement feature in `pkg/actuators/machine/vm.go`
4. Add provider config fields if user-configurable
5. Write unit tests with mocked Nutanix client
6. Update documentation

## Testing Strategy

### Unit Tests
- Use Ginkgo/Gomega framework (existing pattern in codebase)
- Test files: `*_test.go` alongside implementation files
- Mock external dependencies (Nutanix client, Kubernetes client)
- Focus on business logic, error handling, and edge cases

### E2E Tests
- Require actual Nutanix infrastructure
- Run with `make test-e2e`
- Test full machine lifecycle: create → ready → delete

## Code Patterns and Standards

### Error Handling
Always wrap errors with context using `fmt.Errorf` with `%w` verb:
```go
if err := createVM(ctx, vmSpec); err != nil {
    return fmt.Errorf("failed to create VM for machine %s: %w", machine.Name, err)
}
```

### Logging
Use `klog` with machine name for traceability:
```go
klog.Infof("%s: creating machine", machine.Name)
klog.Errorf("failed to create VM for machine %s: %v", machine.Name, err)
```

### Reconciliation
All reconciliation logic must be idempotent:
- Check if resource exists before creating
- Handle already-exists errors gracefully
- Update provider status on every reconciliation
- Use finalizers for cleanup on deletion

### Kubernetes API Conventions
- **Spec**: User's desired state (never modify in controller except defaulting)
- **Status**: Observed state (controller's output)
- Always update status conditions to reflect current state
- Use proper condition types: MachineCreation, InstanceReady

## Environment Configuration

### Required Environment Variables
```bash
NUTANIX_PRISM_CENTRAL_ENDPOINT  # Prism Central hostname/IP
NUTANIX_PRISM_CENTRAL_PORT      # API port (default: 9440)
NUTANIX_PRISM_CENTRAL_USER      # Username for authentication
NUTANIX_PRISM_CENTRAL_PASSWORD  # Password for authentication
```

### Feature Gates
- `FeatureGateMachineAPIMigration`: Controls machine API migration behavior

## Known Constraints and Limitations

**VM Immutability**: Most VM properties are immutable after creation. Updates typically require machine recreation.

**UUID Requirements**: Cluster, subnet, and image references use UUIDs. These must be valid and accessible in the target Nutanix cluster.

**Credentials**: Must be pre-configured in environment variables or Kubernetes secrets. No support for dynamic credential rotation during runtime.

**Network Connectivity**: Requires stable network connectivity to Prism Central. Network interruptions may cause reconciliation failures.

**Prism Central API**: Only supports Prism Central API v3. Does not support Prism Element direct access.

## Integration Points

### With OpenShift
- Deployed by machine-api-operator (typically in `openshift-machine-api` namespace)
- Watches Machine and MachineSet resources across namespaces (configurable via `--namespace` flag)
- Credentials managed via Kubernetes Secrets
- Uses OpenShift's machine-api controller framework

### With Nutanix
- Authenticates with Prism Central using username/password
- Creates VMs in specified Nutanix clusters
- Attaches VMs to specified subnets
- Injects cloud-init user data for node bootstrapping
- Manages VM power state and lifecycle

## Troubleshooting Guide

### Authentication Failures
**Symptoms**: Machines stuck in provisioning, auth errors in logs
**Solution**: Verify `NUTANIX_PRISM_CENTRAL_*` credentials in environment or secret

### VM Creation Failures
**Symptoms**: Machine stuck in provisioning, creation errors in logs
**Solution**:
- Verify cluster UUID exists and is accessible
- Check subnet UUID is valid
- Ensure image is available in target cluster
- Review Prism Central API logs

### Network Issues
**Symptoms**: Timeout errors, connection refused
**Solution**:
- Verify connectivity to Prism Central endpoint
- Check firewall rules for port 9440
- Ensure CA bundle is configured if using HTTPS with custom certs

## Key Files Reference

**Entry Point**: `cmd/manager/main.go` - Controller initialization and setup

**Machine Lifecycle**:
- `pkg/actuators/machine/actuator.go` - Actuator interface
- `pkg/actuators/machine/reconciler.go` - Reconciliation logic
- `pkg/actuators/machine/vm.go` - VM operations

**Configuration**: `pkg/actuators/machine/machine_scope.go` - Machine scope and provider spec access

**Validation**: `pkg/actuators/machineset/controller.go` - MachineSet validation

**API Client**: `pkg/client/client.go` - Nutanix client wrapper

**API Types**: Defined in vendor/github.com/openshift/api/machine/v1beta1 (NutanixMachineProviderConfig, NutanixMachineProviderStatus)

## Development Dependencies

**Core Dependencies**:
- Go 1.24.0+
- controller-runtime (Kubernetes controller framework)
- prism-go-client v0.5.4 (Nutanix API client)
- OpenShift machine-api-operator interfaces

**Build Dependencies**:
- controller-tools (code generation)
- golang/mock (mock generation)

**Testing Dependencies**:
- Ginkgo/Gomega (testing framework)
- envtest (simulated Kubernetes API server)

## Best Practices for AI Agents

When working in this codebase:

1. **Always read relevant files first** before suggesting changes
2. **Follow existing patterns** - this is a mature codebase with established conventions
3. **Write tests** - unit tests are required for all new functionality
4. **Run checks** - always run `make check` before considering work complete
5. **Handle errors properly** - wrap all errors with context
6. **Log with context** - include machine name in all log messages
7. **Keep reconciliation idempotent** - operations must be safe to retry
8. **Update status conditions** - reflect machine state in status
9. **Don't over-engineer** - make minimal necessary changes
10. **Security first** - never log credentials, validate all user inputs

