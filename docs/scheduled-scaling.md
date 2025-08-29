# Scheduled Scaling Feature

## Overview

The Scheduled Scaling feature allows users to proactively scale their applications before anticipated load increases occur. This is particularly useful for predictable traffic patterns such as:

- **TV commercials or marketing campaigns** that drive traffic spikes
- **Push notifications** that trigger user engagement
- **Scheduled events** (e.g., product launches, sales)
- **Load testing** in development environments
- **Seasonal traffic patterns** (e.g., holiday shopping, tax season)

Instead of waiting for the load to increase and then scaling reactively, Scheduled Scaling enables proactive scaling based on user-defined schedules.

## Architecture

### High-Level Flow

```
ScheduledScaling Resource → ScheduledScaling Controller → Tortoise Resource → Tortoise Controller → HPA → Pod Scaling
```

### Component Responsibilities

1. **ScheduledScaling Controller**: Manages time-based scheduling logic and applies scaling configurations to Tortoise resources
2. **Tortoise Controller**: Translates Tortoise policies into HPA configurations (existing functionality)
3. **HPA Controller**: Performs actual pod scaling based on HPA configuration (Kubernetes built-in)

### Design Principles

- **Separation of Concerns**: Each controller has a single responsibility
- **API-First Design**: Controllers communicate only through Kubernetes APIs
- **Limited Blast Radius**: Issues in scheduled scaling don't affect core Tortoise functionality
- **Testability**: Each component can be tested independently

## API Reference

### ScheduledScaling Resource

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: example-scheduled-scaling
  namespace: default
spec:
  schedule:
    startAt: "2024-01-15T10:00:00Z"
    finishAt: "2024-01-15T18:00:00Z"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 5
      minAllocatedResources:
        cpu: "500m"
        memory: "1Gi"
```

### Field Descriptions

#### Schedule
- **startAt**: ISO 8601 timestamp when scaling should begin
- **finishAt**: ISO 8601 timestamp when scaling should end and normal configuration restored

#### TargetRefs
- **tortoiseName**: Name of the Tortoise resource to apply scheduled scaling to

#### Strategy
- **static**: Static scaling configuration (future versions may support dynamic strategies)
  - **minimumMinReplicas**: Minimum number of replicas during the scheduled period
  - **minAllocatedResources**: Minimum resource allocation per pod during the scheduled period

### Status Fields

```yaml
status:
  phase: "Active"  # Pending, Active, Completed, Failed
  lastTransitionTime: "2024-01-15T10:00:00Z"
  reason: "Active"
  message: "Scheduled scaling is currently active"
```

## Usage Examples

### Basic Scheduled Scaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: black-friday-scaling
  namespace: ecommerce
spec:
  schedule:
    startAt: "2024-11-29T00:00:00Z"    # Black Friday start
    finishAt: "2024-11-30T23:59:59Z"   # Black Friday end
  targetRefs:
    tortoiseName: "ecommerce-tortoise"
  strategy:
    static:
      minimumMinReplicas: 20
      minAllocatedResources:
        cpu: "1000m"
        memory: "2Gi"
```

### TV Commercial Scaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: tv-commercial-scaling
  namespace: marketing
spec:
  schedule:
    startAt: "2024-01-20T19:55:00Z"    # 5 minutes before commercial
    finishAt: "2024-01-20T20:05:00Z"   # 5 minutes after commercial
  targetRefs:
    tortoiseName: "marketing-tortoise"
  strategy:
    static:
      minimumMinReplicas: 15
      minAllocatedResources:
        cpu: "800m"
        memory: "1.5Gi"
```

### Load Testing in Development

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: load-test-scaling
  namespace: dev
spec:
  schedule:
    startAt: "2024-01-22T14:00:00Z"    # Load test start
    finishAt: "2024-01-22T16:00:00Z"   # Load test end
  targetRefs:
    tortoiseName: "dev-tortoise"
  strategy:
    static:
      minimumMinReplicas: 10
      minAllocatedResources:
        cpu: "500m"
        memory: "1Gi"
```

## How It Works

### 1. Pre-Schedule Phase
- Controller calculates time until scaling should begin
- Sets status to `Pending`
- Requeues reconciliation at the start time

### 2. Active Scaling Phase
- Controller applies scheduled scaling configuration to the target Tortoise
- Sets status to `Active`
- Requeues reconciliation at the finish time

### 3. Completion Phase
- Controller restores original Tortoise configuration
- Sets status to `Completed`

### 4. Error Handling
- If any phase fails, status is set to `Failed`
- Error details are captured in `reason` and `message` fields

## Implementation Details

### Controller Logic

The ScheduledScaling controller follows this reconciliation flow:

```go
func (r *ScheduledScalingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch ScheduledScaling resource
    // 2. Parse schedule times
    // 3. Determine current phase based on time
    // 4. Apply appropriate scaling strategy
    // 5. Update status and requeue if needed
}
```

### Resource Management

- **Original Configuration Storage**: Original Tortoise configuration is stored in annotations before applying scheduled scaling
- **Conflict Resolution**: Scheduled scaling takes precedence over other scaling policies during the active period
- **Cleanup**: Original configuration is restored when the schedule ends

### RBAC Requirements

The controller requires these permissions:
- `scheduledscalings`: Full CRUD operations
- `tortoises`: Read and update operations
- `scheduledscalings/status`: Status update operations

## Testing

### Unit Tests

Run the controller tests:
```bash
make test
```

### Integration Tests

Deploy to a test cluster and verify:
1. ScheduledScaling resource creation
2. Controller reconciliation
3. Tortoise resource modification
4. HPA scaling behavior
5. Configuration restoration

### Test Scripts

Use the provided test scripts:
- `scripts/test-scheduled-scaling.sh`: Deploy test resources and verify behavior
- `scripts/monitor-scheduled-scaling.sh`: Monitor resources and controller logs

## Deployment

### Prerequisites

1. Tortoise operator deployed and running
2. Kubernetes cluster with HPA support
3. Target applications with Tortoise resources configured

### Installation

1. Apply CRDs:
```bash
kubectl apply -f config/crd/bases/
```

2. Deploy the controller:
```bash
make deploy
```

3. Verify deployment:
```bash
kubectl get pods -n tortoise-system
```

## Monitoring and Troubleshooting

### Controller Logs

```bash
kubectl logs -n tortoise-system deployment/tortoise-controller-manager -c manager
```

### Resource Status

```bash
kubectl get scheduledscalings -A
kubectl describe scheduledscaling <name> -n <namespace>
```

### Common Issues

1. **Invalid Schedule Times**: Ensure startAt is before finishAt and both are valid ISO 8601 timestamps
2. **Missing Tortoise**: Verify the target Tortoise resource exists
3. **RBAC Issues**: Check controller permissions for tortoises and scheduledscalings
4. **Time Zone Confusion**: All times are in UTC, convert local times accordingly

## Future Enhancements

### Planned Features

- **Dynamic Strategies**: CPU/memory-based scaling instead of static values
- **Recurring Schedules**: Daily, weekly, or monthly recurring scaling patterns
- **Multi-Resource Targeting**: Scale multiple Tortoise resources simultaneously
- **Policy Templates**: Reusable scaling configurations
- **Metrics Integration**: Scale based on historical traffic patterns
- **Webhook Notifications**: Notify external systems of scaling events

### API Evolution

The feature is currently in `v1alpha1`, indicating it's experimental. Future versions may include:
- Breaking changes to improve the API design
- Additional validation rules
- Enhanced status fields
- Migration guides between versions

## Contributing

### Development Setup

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run linting and tests
6. Submit a pull request

### Code Generation

After modifying API types:
```bash
make generate    # Generate deepcopy methods
make manifests   # Generate CRDs and RBAC
```

### Testing Guidelines

- Write unit tests for new controller logic
- Test edge cases (invalid schedules, missing resources)
- Verify error handling and status updates
- Test configuration restoration

## Support

For questions or issues:
- Create a GitHub issue
- Tag with `area/scheduled-scaling`
- Provide reproduction steps and logs
- Include cluster and operator versions

## References

- [Tortoise Documentation](../README.md)
- [Kubernetes HPA Documentation](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
