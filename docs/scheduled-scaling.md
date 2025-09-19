# Scheduled Scaling Feature

## Overview

The Scheduled Scaling feature allows users to proactively scale their applications before anticipated load increases occur. This is particularly useful for predictable traffic patterns such as:

- **TV commercials or marketing campaigns** that drive traffic spikes
- **Push notifications** that trigger user engagement
- **Scheduled events** (e.g., product launches, sales)
- **Load testing** in development environments
- **Seasonal traffic patterns** (e.g., holiday shopping, tax season)

Instead of waiting for the load to increase and then scaling reactively, Scheduled Scaling enables proactive scaling based on user-defined schedules.

## Schedule Types

### Time-Based Scheduling
Specify exact start and end times for one-time scaling events.

### Cron-Based Scheduling
Use cron expressions for recurring scaling patterns with configurable duration and timezone support.

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

### Time-Based ScheduledScaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: example-time-scaling
  namespace: default
spec:
  schedule:
    type: time
    startAt: "2024-01-15T10:00:00Z"
    finishAt: "2024-01-15T18:00:00Z"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 15
      minAllocatedResources:
        cpu: "500m"
        memory: "1Gi"
      containerMinAllocatedResources:
      - containerName: "app"
        resources:
          cpu: "800m"
          memory: "2Gi"
```

### Cron-Based ScheduledScaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: example-cron-scaling
  namespace: default
spec:
  schedule:
    type: cron
    cronExpression: "0 9 * * 1-5"  # 9 AM on weekdays
    duration: "8h"                 # Active for 8 hours
    timeZone: "Asia/Tokyo"         # Timezone support
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 20
      minAllocatedResources:
        cpu: "1000m"
        memory: "2Gi"
```

### Field Descriptions

#### Schedule
- **type**: `time` or `cron` - Type of scheduling
- **startAt**: (time only) ISO 8601 timestamp when scaling should begin
- **finishAt**: (time only) ISO 8601 timestamp when scaling should end
- **cronExpression**: (cron only) Standard cron expression (e.g., `"0 9 * * 1-5"`)
- **duration**: (cron only) How long each scaling window lasts (e.g., `"8h"`, `"30m"`)
- **timeZone**: (cron only) Timezone for cron scheduling (default: `"Asia/Tokyo"`)

#### TargetRefs
- **tortoiseName**: Name of the Tortoise resource to apply scheduled scaling to

#### Strategy
- **static**: Static scaling configuration
  - **minimumMinReplicas**: Minimum number of replicas during the scheduled period
  - **minAllocatedResources**: Global minimum resource allocation per container
    - **cpu**: CPU request (e.g., `"500m"`)
    - **memory**: Memory request (e.g., `"1Gi"`)
  - **containerMinAllocatedResources**: Container-specific resource overrides
    - **containerName**: Name of the container to override
    - **resources**: Resource specifications for this container

### Status Fields

```yaml
status:
  phase: "Active"  # Pending, Active, Completed, Failed
  lastTransitionTime: "2024-01-15T10:00:00Z"
  reason: "Active"
  message: "Scheduled scaling is currently active"
  humanReadableSchedule: "Every day at 9:00 AM for 8 hours"
  formattedStartTime: "Today at 9:00 AM"
  formattedEndTime: "Today at 5:00 PM"
  formattedNextStartTime: "Tomorrow at 9:00 AM"
```

## Usage Examples

### Cron-Based Recurring Scaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: business-hours-scaling
  namespace: production
spec:
  schedule:
    type: cron
    cronExpression: "0 9 * * 1-5"  # 9 AM on weekdays
    duration: "8h"                 # Active for 8 hours (9 AM - 5 PM)
    timeZone: "Asia/Tokyo"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 20
      minAllocatedResources:
        cpu: "1000m"
        memory: "2Gi"
```

### One-Time Event Scaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: tv-commercial-scaling
  namespace: marketing
spec:
  schedule:
    type: time
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

### Container-Specific Scaling

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: mixed-container-scaling
  namespace: production
spec:
  schedule:
    type: cron
    cronExpression: "*/10 * * * *"  # Every 10 minutes (for testing)
    duration: "5m"
    timeZone: "Asia/Tokyo"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 10
      minAllocatedResources:        # Global defaults
        cpu: "500m"
        memory: "1Gi"
      containerMinAllocatedResources:  # Container-specific overrides
      - containerName: "web-server"
        resources:
          cpu: "1000m"
          memory: "2Gi"
      - containerName: "sidecar"
        resources:
          cpu: "200m"
          memory: "512Mi"
```

## How It Works

### 1. Pending Phase
- Controller calculates time until scaling should begin
- Sets status to `Pending` with next window information
- Requeues reconciliation at the start time

### 2. Active Phase
- Controller applies scheduled scaling configuration to the target Tortoise:
  - Sets resource requests (CPU/Memory) on containers
  - Sets HPA minReplicas through Tortoise annotation
  - Stores original configuration for restoration
- Sets status to `Active`
- Requeues reconciliation at the finish time

### 3. Restoration Phase
- Controller restores original Tortoise configuration
- Removes ScheduledScaling annotations
- HPA minReplicas returns to original value
- Sets status to `Pending` (for cron) or `Completed` (for time-based)

### 4. Emergency Mode Protection
- Extended grace period (10 minutes vs 5 minutes) during ScheduledScaling
- Emergency mode respects ScheduledScaling minReplicas when both are active
- Prevents false emergency triggers during rapid scaling

### 5. Error Handling
- Robust conflict resolution with retry logic
- Finalizer-based cleanup ensures proper restoration on deletion
- Comprehensive error logging and status updates

## Key Features

### Resource Scaling
- **Global Resource Requests**: Apply CPU/memory minimums to all containers
- **Container-Specific Overrides**: Set different resources per container
- **HPA MinReplicas**: Dynamically adjust minimum replica count
- **Consistent Scaling**: Ensures maxReplicas ≥ minReplicas

### Schedule Management
- **Time-Based**: One-time scaling events with start/end times
- **Cron-Based**: Recurring scaling with cron expressions
- **Timezone Support**: Configure schedules in local timezones
- **Human-Readable Status**: Clear schedule descriptions in status

### Emergency Mode Integration
- **Extended Grace Period**: 10-minute grace period during ScheduledScaling (vs 5 minutes normally)
- **Metric Failure Protection**: Handles all types of HPA metric failures
- **Smart Emergency Override**: Emergency mode respects ScheduledScaling minReplicas
- **Empty Metrics Handling**: Prevents false emergencies during rapid scaling

### Reliability Features
- **Finalizer-Based Cleanup**: Ensures proper restoration even if ScheduledScaling is deleted
- **Conflict Resolution**: Automatic retry with exponential backoff
- **Original State Preservation**: Stores and restores complete Tortoise configuration
- **Annotation-Based Communication**: Clean integration with existing Tortoise controller

## Testing

### Quick Test

Use the provided test script:
```bash
# Test cron-based scheduling (default)
ASSERT=1 ./scripts/test-scheduled-scaling.sh

# Test time-based scheduling
SCHEDULE_TYPE=time ASSERT=1 ./scripts/test-scheduled-scaling.sh
```

### Manual Testing

```bash
# Check ScheduledScaling status
kubectl get scheduledscalings -A

# Check Tortoise and HPA changes
kubectl get tortoise <tortoise-name> -o yaml
kubectl get hpa <hpa-name> -o yaml

# Monitor controller logs
kubectl logs -n mercari-tortoise-lab deployment/tortoise-controller-manager -f
```

## Troubleshooting

### Common Issues

1. **Status Not Updating**: Ensure ScheduledScaling controller is registered in `main.go`
2. **Emergency Mode Override**: Check HPA conditions for metric failures; ScheduledScaling extends grace period automatically
3. **Resource Not Restored**: Finalizer ensures cleanup even if ScheduledScaling is force-deleted
4. **HPA MinReplicas Not Applied**: Verify Tortoise has HPA reference and annotation is set
5. **Conflict Errors**: Controller automatically retries with latest resource version

### Controller Logs

```bash
# Check ScheduledScaling controller logs
kubectl logs -n mercari-tortoise-lab deployment/tortoise-controller-manager -f | grep scheduledscaling

# Check Tortoise controller logs for HPA updates
kubectl logs -n mercari-tortoise-lab deployment/tortoise-controller-manager -f | grep -i hpa
```

### Debugging Commands

```bash
# Check ScheduledScaling status
kubectl get scheduledscalings -A
kubectl describe scheduledscaling <name> -n <namespace>

# Check Tortoise annotations
kubectl get tortoise <name> -o yaml | grep -A5 -B5 scheduledscaling

# Check HPA current state
kubectl get hpa <name> -o yaml
```

## Notes

- **Timezone Default**: All schedules default to `Asia/Tokyo` timezone
- **Grace Period**: Extended to 10 minutes during ScheduledScaling to prevent false emergency mode triggers
- **Annotation-Based**: Uses Tortoise annotations for clean integration with existing controller
- **Conflict Resolution**: Automatic retry logic handles concurrent updates
- **Emergency Mode**: Respects ScheduledScaling minReplicas even when in emergency mode
