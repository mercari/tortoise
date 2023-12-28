## User guide

<img alt="Tortoise" src="images/tortoise.jpg" width="400px"/>

This page describes a minimum knowledge that the end-users have to know, 
and how they can configure Tortoise so that they can let tortoises autoscale their workloads.

### How tortoise works

Actually, Tortoise itself doesn't directly change your Pod's resource request or the number of replicas.
It has HorizontalPodAutoscaler and VerticalPodAutoscaler under the hood, 
and your tortoise just keeps updating them to be well-optimized based on your workload's historical resource usage.

### Configuration overview

Tortoise is designed to be a very simple configuration:

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
metadata:
  name: lovely-tortoise
  namespace: zoo
spec:
  updateMode: Auto # enable autoscaling.
  targetRefs:      # which workload this tortoise autoscales.
    scaleTargetRef:
      kind: Deployment
      name: sample 
```

This is the example for a minimum required configuration. 

### updateMode

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
spec:
...
  updateMode: Auto 
```

`.spec.updateMode` could contain three values:
- `Off` (default): DryRun mode. The tortoise doesn't change anything in your workload or autoscaler.
- `Auto`: The tortoise keep updating your workload or autoscaler to be optimized.
- `Emergency`: The tortoise scale up/out your workload to be big enough so that the workload can handle unexpectedly bigger traffic.

#### updateMode: `Off`

`Off` is the default value of `updateMode`. 
It means a DryRun mode - the tortoise doesn't change anything in your workload or autoscaler.

But, even during `Off` mode, the tortoise actually generates the recommendation for your workload's resource request, and your HPA's target utilization.

You can observe the recommendation values with these metrics:
- `mercari.tortoise.proposed_cpu_request`: CPU request a tortoise proposes.
- `mercari.tortoise.proposed_memory_request`: memory request that a tortoise proposes.
- `mercari.tortoise.proposed_hpa_minreplicas`: HPA `.spec.minReplicas` that a tortoise proposes.
- `mercari.tortoise.proposed_hpa_maxreplicas`: HPA `.spec.maxReplicas` that a tortoise proposes.
- `mercari.tortoise.proposed_hpa_utilization_target`: HPA `.spec.metrics[*].containerResource.target.averageUtilization` that a tortoise proposes.

#### updateMode: `Auto`

`Auto` is a update mode to let tortoise keep updating your workload or autoscaler to be optimized.

#### updateMode: `Emergency`

`Emergency` is a update mode to enable the emergency mode.
Please refer to [Emergency mode](./emergency.md) for more details.

### `.spec.AutoscalingPolicy`

There are two primary options for configuring resource scaling within containers:
1. Allow Tortoise to automatically determine the appropriate autoscaling policy for each resource.
2. Manually define the autoscaling policy for each resource.

The AutoscalingPolicy field is mutable; you can modify it at any time, whether from an empty state to populated or vice versa.

#### 1. Allow Tortoise to automatically determine the appropriate autoscaling policy for each resource

To do this, you simply leave `.spec.AutoscalingPolicy` unset. 

In this case, Tortoise will adjust the autoscaling policies using the following logic:
- If `.spec.TargetRefs.HorizontalPodAutoscalerName` is not provided, the policies default to "Horizontal" for CPU and "Vertical" for memory across all containers.
- If `.spec.TargetRefs.HorizontalPodAutoscalerName` is specified, resources governed by the referenced Horizontal Pod Autoscaler will use a "Horizontal" policy,
while those not managed by the HPA will use a "Vertical" policy.
Note that Tortoise supports only the `ContainerResource` metric type for HPAs; other metric types will be disregarded.
Additionally, if a `ContainerResource` metric is later added to an HPA associated with Tortoise,
Tortoise will automatically update relevant resources to utilize a `Horizontal` policy in AutoscalingPolicy.
- if a container doesn't have the resource request, that container's autoscaling policy is always set to "Off" because tortoise cannot generate any recommendation without the resource request.

#### 2. Manually define the autoscaling policy for each resource.

With the second option, you must manually specify the AutoscalingPolicy for the resources of each container within this field.

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
spec:
...
  autoscalingPolicy: 
    - containerName: istio-proxy
      policy:
        cpu: Horizontal
        memory: Vertical
    - containerName: app
      policy:
        cpu: Horizontal
        memory: Vertical
```

AutoscalingPolicy is an optional field for specifying the scaling approach for each resource within each container.
- `Horizontal`: Tortoise increases the replica number when the resource utilization goes up.
- `Vertical`: Tortoise scales up the resource given to the container when the resource utilization goes up.
- `Off`(default): Tortoise doesn't look at the resource of the container at all. 

If policies are defined for some but not all containers or resources, Tortoise will assign a default `Off` policy to unspecified resources.
Be aware that when new containers are introduced to the workload, the AutoscalingPolicy configuration must be manually updated 
if you want to configure autoscaling for a new container,
as Tortoise will default to an `Off` policy for resources within the new container, preventing scaling.

### `.spec.DeletionPolicy`

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
spec:
...
  deletionPolicy: "DeleteAll"
```

DeletionPolicy is the policy how the controller deletes associated HPA and VPAs when tortoise is removed.

- `DeleteAll`: tortoise deletes all associated HPA and VPAs, created by tortoise. 
But, if the associated HPA is not created by tortoise, that is associated by `spec.targetRefs.horizontalPodAutoscalerName`, 
tortoise doesn't delete the HPA even with `DeleteAll`.
- `NoDelete`(default): tortoise doesn't delete any associated HPA and VPAs.

### `.spec.ResourcePolicy`

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
spec:
...
  resourcePolicy:
    - containerName: istio-proxy
      minAllocatedResources:
        cpu: "4"
```

ResourcePolicy contains the policy how each resource is updated.
It currently only contains `minAllocatedResources` to indicate the minimum amount of resources which is given to the container.
e.g., if `minAllocatedResources` is configured as the above example, Tortoise won't set cpu smaller than `4` in `istio-proxy` container
even if the autoscaling policy for `istio-container` cpu is `Vertical` and VPA suggests changing cpu smaller than `4`.
