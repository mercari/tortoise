# Scheduled Scaling Testing Guide

This guide provides comprehensive instructions for testing the Scheduled Scaling functionality in Tortoise.

## Overview

Scheduled Scaling allows users to predict and prepare for increased resource consumption by scheduling scaling operations in advance. This is particularly useful for:

- **TV Campaigns**: When you know traffic will spike at specific times
- **Push Notifications**: Before sending notifications that will drive user engagement
- **Load Testing**: In development environments to simulate production loads
- **Scheduled Events**: Any predictable increase in resource demand

## Architecture

The Scheduled Scaling feature consists of:

1. **ScheduledScaling CRD** (`autoscaling.mercari.com/v1alpha1`)
2. **ScheduledScaling Controller** - Manages the lifecycle and timing
3. **Integration with Tortoise** - Applies scaling policies to existing Tortoise resources

## Testing Phases

### Phase 1: Local Development Testing

#### Prerequisites
- Go 1.22+
- kubectl configured
- kind or minikube cluster
- Docker

#### Setup
```bash
# Clone the repository
git clone <repository-url>
cd tortoise

# Install dependencies
go mod tidy

# Generate code
make generate
make manifests

# Build the controller
make build
```

#### Run Tests
```bash
# Run unit tests
make test

# Run specific controller tests
go test ./internal/controller/ -v

# Run with coverage
make test-coverage
```

### Phase 2: Integration Testing

#### Deploy to Local Cluster
```bash
# Deploy CRDs
make install

# Deploy the controller
make deploy

# Verify deployment
kubectl get pods -n tortoise-system
```

#### Test Basic Functionality
```bash
# Apply example resources
kubectl apply -f test/scheduled-scaling-example.yaml

# Check resource status
kubectl get scheduledscalings
kubectl get tortoises
kubectl get deployments
```

### Phase 3: End-to-End Testing

#### Use the Test Script
```bash
# Make script executable
chmod +x scripts/test-scheduled-scaling.sh

# Run comprehensive test
./scripts/test-scheduled-scaling.sh
```

#### Monitor in Real-Time
```bash
# Use monitoring script
chmod +x scripts/monitor-scheduled-scaling.sh
./scripts/monitor-scheduled-scaling.sh default 5
```

## Testing Scenarios

### Scenario 1: Basic Scheduled Scaling

**Objective**: Verify that a ScheduledScaling resource can be created and managed.

**Steps**:
1. Create a ScheduledScaling with start time 1 minute in the future
2. Verify status transitions: Pending → Active → Completed
3. Check that Tortoise resources are updated accordingly

**Expected Results**:
- Status phase transitions correctly
- Tortoise scaling policies are applied during active period
- Resources return to normal after completion

### Scenario 2: Invalid Schedule Validation

**Objective**: Ensure the controller properly validates schedule times.

**Steps**:
1. Create a ScheduledScaling with finish time before start time
2. Verify the resource enters Failed state
3. Check error messages and reasons

**Expected Results**:
- Resource status becomes Failed
- Appropriate error reason is set
- Controller logs show validation errors

### Scenario 3: Resource Conflict Handling

**Objective**: Test behavior when multiple scaling policies conflict.

**Steps**:
1. Create a ScheduledScaling that overlaps with existing Tortoise policies
2. Verify graceful handling of conflicts
3. Check that the most appropriate policy is applied

**Expected Results**:
- No resource corruption
- Clear logging of policy decisions
- Graceful fallback behavior

### Scenario 4: Time Zone Handling

**Objective**: Verify proper handling of different time zones.

**Steps**:
1. Create ScheduledScaling resources with various time zone formats
2. Test edge cases around midnight and DST transitions
3. Verify consistent behavior across time zones

**Expected Results**:
- Consistent behavior regardless of time zone
- Proper handling of edge cases
- Clear time format requirements

## Verification Commands

### Check Resource Status
```bash
# ScheduledScaling resources
kubectl get scheduledscalings -A -o wide

# Tortoise resources
kubectl get tortoises -A -o wide

# Deployment status
kubectl get deployments -A -o wide

# HPA status
kubectl get hpa -A -o wide
```

### View Detailed Information
```bash
# ScheduledScaling details
kubectl describe scheduledscaling <name> -n <namespace>

# Tortoise details
kubectl describe tortoise <name> -n <namespace>

# Events
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
```

### Monitor Controller Logs
```bash
# Controller logs
kubectl logs -n tortoise-system deployment/tortoise-controller-manager -f

# Filter for scheduled scaling events
kubectl logs -n tortoise-system deployment/tortoise-controller-manager | grep -i "scheduled"
```

## Troubleshooting

### Common Issues

#### 1. Controller Not Starting
```bash
# Check controller status
kubectl get pods -n tortoise-system

# Check logs
kubectl logs -n tortoise-system deployment/tortoise-controller-manager

# Verify CRDs are installed
kubectl get crd | grep scheduledscaling
```

#### 2. Resources Not Updating
```bash
# Check controller reconciliation
kubectl logs -n tortoise-system deployment/tortoise-controller-manager | grep -i "reconciling"

# Verify resource ownership
kubectl get scheduledscaling <name> -o yaml | grep -A 5 "ownerReferences"

# Check for finalizers
kubectl get scheduledscaling <name> -o yaml | grep -A 5 "finalizers"
```

#### 3. Time Parsing Errors
```bash
# Verify time format
kubectl get scheduledscaling <name> -o yaml | grep -A 5 "schedule"

# Check controller logs for parsing errors
kubectl logs -n tortoise-system deployment/tortoise-controller-manager | grep -i "time\|parse"
```

### Debug Mode

Enable debug logging for more detailed information:

```bash
# Update deployment with debug log level
kubectl patch deployment tortoise-controller-manager -n tortoise-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"manager","env":[{"name":"LOG_LEVEL","value":"DEBUG"}]}]}}}}'

# Restart the controller
kubectl rollout restart deployment/tortoise-controller-manager -n tortoise-system
```

## Performance Testing

### Load Testing
```bash
# Create multiple ScheduledScaling resources
for i in {1..10}; do
  kubectl apply -f - <<EOF
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: load-test-$i
  namespace: default
spec:
  schedule:
    startAt: "$(date -d "+$i minutes" -u +"%Y-%m-%dT%H:%M:%SZ")"
    finishAt: "$(date -d "+$((i+1)) minutes" -u +"%Y-%m-%dT%H:%M:%SZ")"
  targetRefs:
    tortoiseName: test-tortoise
  strategy:
    static:
      minimumMinReplicas: 3
      minAllocatedResources:
        cpu: "500m"
        memory: "512Mi"
EOF
done
```

### Stress Testing
```bash
# Monitor resource usage during stress test
kubectl top pods -n tortoise-system
kubectl top nodes

# Check controller performance metrics
kubectl get --raw /metrics | grep -i "controller"
```

## Advanced Scenarios

### Scenario 5: Rolling Updates During Scaling

**Objective**: Test behavior when deployments are updated during scheduled scaling.

**Steps**:
1. Start a ScheduledScaling operation
2. Trigger a rolling update on the target deployment
3. Verify scaling continues correctly

### Scenario 6: Cluster Scaling Events

**Objective**: Test behavior during cluster-level scaling events.

**Steps**:
1. Create a ScheduledScaling operation
2. Scale the cluster nodes up/down
3. Verify scheduled scaling adapts appropriately

### Scenario 7: Network Partition Recovery

**Objective**: Test recovery from network partitions.

**Steps**:
1. Start a ScheduledScaling operation
2. Simulate network partition
3. Verify proper recovery and state consistency

## Cleanup

### Remove Test Resources
```bash
# Remove ScheduledScaling resources
kubectl delete scheduledscalings --all -n default

# Remove Tortoise resources
kubectl delete tortoises --all -n default

# Remove test deployments
kubectl delete deployment test-deployment -n default

# Remove test namespace
kubectl delete namespace test-namespace
```

### Uninstall Controller
```bash
# Remove controller deployment
make undeploy

# Remove CRDs
make uninstall
```

## Best Practices

1. **Always test in non-production environments first**
2. **Use realistic time schedules for testing**
3. **Monitor resource usage during tests**
4. **Clean up test resources promptly**
5. **Document any issues or unexpected behaviors**
6. **Test edge cases and error conditions**
7. **Verify integration with existing Tortoise functionality**

## Support

For issues or questions:

1. Check the troubleshooting section above
2. Review controller logs for error messages
3. Verify resource configurations
4. Check Kubernetes events for additional context
5. Consult the main Tortoise documentation
6. Open an issue in the project repository

## Future Enhancements

The Scheduled Scaling feature is designed to be extensible. Future enhancements may include:

- **Dynamic Scheduling**: Based on metrics and predictions
- **Recurring Schedules**: Daily, weekly, or monthly patterns
- **Advanced Strategies**: Beyond static scaling policies
- **Integration with External Systems**: Calendar APIs, monitoring systems
- **Multi-Resource Targeting**: Scale multiple resources simultaneously
- **Policy Templates**: Reusable scaling configurations
