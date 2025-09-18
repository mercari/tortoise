# Global Disable Mode

The Global Disable Mode is a feature that allows you to disable Tortoise from applying any recommendations while still allowing it to perform calculations and update its status. This is particularly useful for scenarios like ScaleOps POC where you want to disable Tortoise without modifying individual Tortoise resources.

## How it Works

When Global Disable Mode is enabled:

- **Tortoise continues to run** and perform all calculations
- **Status updates** are still generated and stored in the Tortoise resource
- **Recommendations** are calculated and stored in the status
- **No changes** are applied to HPA, VPA, or Pod resources
- **Metrics** are still collected and exposed
- **Events** are still recorded (but indicate that recommendations are not applied)

## Configuration

### Via Configuration File

Add the following to your Tortoise configuration file:

```yaml
GlobalDisableMode: true
```

### Via Environment Variable

Set the environment variable:

```bash
export TORTOISE_GLOBAL_DISABLE_MODE=true
```

### Default Value

The default value is `false`, meaning Tortoise operates normally unless explicitly disabled.

## Use Cases

### ScaleOps POC

When running a ScaleOps proof of concept, you might want to:
1. Deploy Tortoise to observe its behavior
2. See what recommendations it would make
3. Verify that it's working correctly
4. But not actually apply any changes to your workloads

Global Disable Mode is perfect for this scenario.

### Testing and Validation

Before enabling Tortoise in production:
1. Deploy with Global Disable Mode enabled
2. Monitor the recommendations for a period
3. Validate that the recommendations make sense
4. Once confident, disable Global Disable Mode to start applying changes

### Maintenance Windows

During maintenance or troubleshooting:
1. Enable Global Disable Mode to stop applying changes
2. Perform maintenance tasks
3. Disable Global Disable Mode to resume normal operation

## Monitoring

### Prometheus Metrics

The global disable mode status is exposed via a Prometheus metric:

```
tortoise_global_disable_mode
```

- `1` = Global Disable Mode is enabled
- `0` = Global Disable Mode is disabled

### Status Conditions

When Global Disable Mode is enabled, Tortoise will update the status conditions to indicate that recommendations are not being applied:

```yaml
status:
  conditions:
  - type: VerticalRecommendationUpdated
    status: "False"
    reason: "GlobalDisableModeEnabled"
    message: "The recommendation is not applied because global disable mode is enabled"
```

## Example Configuration

Here's a complete example configuration file with Global Disable Mode enabled:

```yaml
# Global Disable Mode - disables applying recommendations
GlobalDisableMode: true

# Other configuration options
RangeOfMinMaxReplicasRecommendationHours: 1
GatheringDataPeriodType: "weekly"
MaxReplicasRecommendationMultiplier: 2.0
MinReplicasRecommendationMultiplier: 0.5
ReplicaReductionFactor: 0.95
MaximumTargetResourceUtilization: 90
MinimumTargetResourceUtilization: 65
MinimumMinReplicas: 3
PreferredMaxReplicas: 30
MaximumCPURequest: "10"
MinimumCPURequest: "50m"
MaximumMemoryRequest: "10Gi"
MinimumMemoryRequest: "50Mi"
TimeZone: "Asia/Tokyo"
TortoiseUpdateInterval: "15s"
HPATargetUtilizationMaxIncrease: 5
MaximumMinReplicas: 10
MaximumMaxReplicas: 100
MaxAllowedScalingDownRatio: 0.8
BufferRatioOnVerticalResource: 0.1
EmergencyModeGracePeriod: "5m"
```

## Comparison with UpdateMode

| Feature | UpdateMode: Off | Global Disable Mode |
|---------|----------------|-------------------|
| Scope | Per-Tortoise resource | Global (all Tortoise resources) |
| Configuration | In Tortoise spec | In controller configuration |
| Use Case | Individual resource control | System-wide control |
| Status Updates | Yes | Yes |
| Recommendations | Yes | Yes |
| HPA Updates | No | No |
| VPA Updates | No | No |
| Pod Updates | No | No |

## Implementation Details

The Global Disable Mode is implemented at the service level:

- **TortoiseService**: Checks `IsGlobalDisableModeEnabled()` before applying VPA recommendations
- **HpaService**: Checks `IsGlobalDisableModeEnabled()` before updating HPA resources
- **Controller**: Checks `IsGlobalDisableModeEnabled()` before triggering rollout restarts

This ensures that all recommendation applications are consistently disabled when the mode is enabled.
