## Horizontal scaling

<img alt="Tortoise" src="images/horizontal.jpg" width="500px"/>

You can configure resource(s) to be scaled by horizontal scaling
by setting `Horizontal` in `Spec.ResourcePolicy[*].AutoscalingPolicy`

For `Horizontal` resources, Tortoise keeps changing the corresponding HPA's fields with the recommendation value calculated from the historical usage.

### Configure Horizontal scaling

#### Attach your HPA

You can attach your HPA via `.spec.targetRefs.HorizontalPodAutoscalerName`.

Currently, Tortoise supports only `type: ContainerResource` metric. 

If HPA has `type: Resource` metrics, Tortoise just removes them because they'd be conflict with `type: ContainerResource` metrics managed by Tortoise.
If HPA has metrics other than `Resource` or `ContainerResource`, Tortoise just keeps them. 

### How Tortoise 

### MaxReplicas

MaxReplicas is calculated by:

```
# If GatheringDataPeriodType = weekly
max{replica numbers at the same time on the same day of week} * MaxReplicasFactor

# If GatheringDataPeriodType = daily 
max{replica numbers at the same time} * MaxReplicasFactor
```

(refer to [admin-guide.md](./admin-guide.md) about each parameter)

### MinReplicas

MinReplicas is calculated by:

```
# If GatheringDataPeriodType = weekly
max{replica numbers at the same time on the same day of week} * MinReplicasFactor

# If GatheringDataPeriodType = daily
max{replica numbers at the same time} * MinReplicasFactor
```

(refer to [admin-guide.md](./admin-guide.md) about each parameter)

### Target utilization

Target utilization is calculated by:

```
100 - (max{recommended resource usage from VPA}/{current resource request} - {current target utilization})
```

Note: the maximum target utilization is configurable by the cluster admin.

#### Why we can calculate the target utilization from the above formula?

The target utilization should be less than `100%` because
- each container's resource utilization isn't the same; one may be higher, but another may be lower. But, the HPA can only see the average among them.
- the HPA cannot scale up the Pod instantly when the utilization goes higher than `100%`.

Thus, the question "how to calculate the best target utilization" equals 
- how to calculate the expected difference in each container's resource utilization.
  - If the difference is pretty big, we should give more additional resources through the target utilization.
- how to calculate the amount of resources which is needed to handle traffic while waiting for new Pods to be run up.

The VPA's recommendation is generally generated from the P90 past resource usage. 
(See around [here](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler/pkg/recommender/logic) to know more detailed logic)

Looking back the above formula, 
- `max{recommended resource usage from VPA}/{current resource request}` means the resource utilization at P90 resource usage in history
- `max{recommended resource usage from VPA}/{current resource request} - {current target utilization}` means required additional resource which is needed to:
  - handle traffic while waiting for new Pods to be run up.
  - make all container's resource utilization below 100%.
- Thus, finally `100 - (max{recommended resource usage from VPA}/{current resource request} - {current target utilization})` means the target utilization which only give the bare minimum additional resources.

### The container right sizing

Although it says "Horizontal", 
a tortoise possibly changes the container's size if it's too large or too small.
- if the number of replicas equals `MinimumMinReplicas`, make each container's size smaller instead of reducing the number of replicas.
- if the number of replicas equals `PreferredReplicaNumUpperLimit`, make each container's size bigger instead of increasing the number of replicas.
  - But, when the resource request reaches `MaximumCPUCores` or `MaximumMemoryBytes`, tortoise will ignore `PreferredReplicaNumUpperLimit`, and increase the number of replicas.

I know it's complicated, describe specifically in the following examples.

#### Example1: reach `PreferredReplicaNumUpperLimit`

Let's say the global configurations are:
- `MinimumMinReplicas` 3
- `PreferredReplicaNumUpperLimit`: 10
- `MaximumCPUCores`: 5 cores

And, the target workload currently looks like:
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: 1 cores
- resource request: CPU 2 cores
- the num of replicas: 10 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption is increasing.

This case, Tortoise prefers not to increase the replica number more. 
Instead, Tortoise temporary switch the scaling way to Vertical to make each container bigger. 

After a while, the target workload looks like:
- resource request: 4 cores. 
- the num of replicas: 10 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption starts to be decreasing.

Given the resource consumption starts to be decreasing, the Tortoise switch the scaling way back to Horizontal.
So, this workload will continue to work with 4 cores after this time.

If the traffic next day is very similar to this day, then Tortoise no longer needs to switch the scaling way to Vertical during peak time.

#### Example2: reach `PreferredReplicaNumUpperLimit` and `MaximumCPUCores` and `MaximumMemoryBytes`

Let's say the global configurations are (the same as Example1):
- `MinimumMinReplicas` 3
- `PreferredReplicaNumUpperLimit`: 10
- `MaximumCPUCores`: 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 10 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption is increasing.

As described in Example1, Tortoise temporary switches the scaling way to Vertical.

After a while, the target workload looks like:
- resource request: CPU 5 cores (the same value as `MaximumCPUCores` and `MaximumMemoryBytes` given to each resource")
- the num of replicas: 10 (the same as "preferred maximum number of replicas")
- still the resource consumption is increasing.

The resource request reaches the `MaximumCPUCores` and `MaximumMemoryBytes` given to each resource" now.
So, Tortoise switch the scaling way back to the Horizontal.

After a while, the target workload looks like:
- resource request: CPU 5 cores (CPU request is the same value as `MaximumCPUCores` and `MaximumMemoryBytes`)
- the num of replicas: 15 (more than `PreferredReplicaNumUpperLimit`)
- the resource consumption starts to be decreasing.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during peak time
because it's already reached `MaximumCPUCores` and `MaximumMemoryBytes`.

#### Example 3: reach the minimum number of replicas

Let's say the global configurations are (the same as Example1):
- `MinimumMinReplicas` 3
- `PreferredReplicaNumUpperLimit`: 10
- `MaximumCPUCores`: CPU 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 3 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1.5 cores 
- the num of replicas: 3 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption starts to be increasing.

Given the resource consumption starts to be increasing, the Tortoise switches the scaling way back to Horizontal.
So, this workload will continue to work with 1.5 cores after this time.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during off-peak time.

#### Example 4: reach the minimum number of replicas and `.spec.ResourcePolicy[*].MinAllocatedResources`

Let's say the global configurations are (the same as Example1):
- `MinimumMinReplicas` 3
- `PreferredReplicaNumUpperLimit`: 10
- `MaximumCPUCores`: CPU 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 3 (the same as `PreferredReplicaNumUpperLimit`)
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1 cores (the same as `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`)
- the num of replicas: 3 (the same as `PreferredReplicaNumUpperLimit`)
- still the resource consumption is decreasing.

The resource request reaches the `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`.
But, the num of replicas has already reached `PreferredReplicaNumUpperLimit`.

So, Tortoise won't change anything further.

### container size adjustment for the multiple container Pods

Although it says "Horizontal",
again, this optimization makes vertical change in your resource.

If several resources are managed by HPA,
it often happens that one resource always kick HPA to scale up the deployment
while other HPA-managed resources are low utilized.

Tortoise adjusts such low utilized resource's request to be smaller in this case.

#### Example

Let's say:
- The Pod has two containers (app and istio-proxy)
- The app container requests 10 cores, and the istio-proxy container requests 5 cores.
- The both container's CPU scale via HPA. (both targets 80%)
- The app container usually uses around 80% (8 cores), whereas the istio-proxy container usually uses around 40% (2 cores).
- It could happen because the app container always push HPA to scale out the deployment 
while the istio-proxy doesn't need to scale up and always has wasted CPU.

In this case, we can change the CPU request of istio-proxy to 2.5 cores.
Then, the CPU utilization of the istio-proxy is changed to 80% which is around the target utilization of HPA.
