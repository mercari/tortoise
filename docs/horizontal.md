## Horizontal scaling

<img alt="Tortoise" src="images/horizontal.jpg" width="500px"/>

You can configure resource(s) to be scaled by horizontal scaling
by setting `Horizontal` in `Spec.ResourcePolicy[*].AutoscalingPolicy`

For `Horizontal` resources, Tortoise keeps changing the corresponding HPA's fields with the recommendation value calculated from the historical usage.

Let's get into detail how each field gets changed.

### MaxReplicas

MaxReplicas is calculated by:

```
max{replica numbers at the same time on the same weekday} * 2
```

It only takes the num of replicas of the last 4 weeks into consideration.

### MinReplicas

MinReplicas is calculated by:

```
max{replica numbers at the same time on the same weekday} * 1/2
```

It only takes the num of replicas of the last 4 weeks into consideration.

### Target utilization

Target utilization is calculated by:

```
100 - (max{recommended resource usage from VPA}/{current resource request} - {current target utilization})
```

Note: the maximum target utilization is configurable by the cluster admin.

#### Why we can calculate the target utilization from the above formula?

You know, the target utilization should be below `100%`.
Why? mostly because the HPA cannot scale up the Pod instantly when the utilization goes higher than `100%`.

Let's say we configure the target utilization with `70%`, 
`70%` of resources is usually consumed, and the rest `30%` is extra resources which is usually not consumed.
But, extra `30%` is given to the workload 
because the workload need to handle traffic while HPA works on scaling up when the resource utilization goes higher than `77%`. 
(Actually, not only HPA, but also many components are involved to run up new Pods.)

(Why not `70%` but `77%` -> it's because of the [globally-configurable tolerance](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#algorithm-details))

Thus, the question "how to calculate the best target utilization" 
equals "how to calculate the amount of resources which is needed to handle traffic while waiting for new Pods to be run up".

The VPA's recommendation is generally generated from the P90 past resource usage. 
(See around [here](https://github.com/kubernetes/autoscaler/tree/master/vertical-pod-autoscaler/pkg/recommender/logic) to know more detailed logic)

Tortoise assumes that this P90 usage is near the resource usage while waiting for new Pods to be run up.

Looking back the above formula, 
- `max{recommended resource usage from VPA}/{current resource request}` means the resource utilization at P90 resource usage
- `max{recommended resource usage from VPA}/{current resource request} - {current target utilization}` means the additional resource which is needed to handle traffic while waiting for new Pods to be run up.
- Thus, finally `100 - (max{recommended resource usage from VPA}/{current resource request} - {current target utilization})` means the target utilization which only give the bare minimum additional resources.

#### Supported metrics in HPA

Basically, Tortoise only touches `type: ContainerResource`; check each container's situation and adjust the target utilization of each of them.

But, given [the container resource metrics](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#container-resource-metrics) is still alpha as of v1.26, 
some companies are using the external metrics and the custom metrics to fetch the container's resource utilization.

So, as a temporal solution, Tortoise supports the external metrics which refers to the container's resource utilization.

To let tortoise regard the external metrics as the container resource metrics,
you need to give the annotation to your HPA like:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: example-hpa
  namespace: example-prod
  annotations:
    tortoises.autoscaling.mercari.com/container-based-cpu-metric-prefix: "datadogmetric@example-prod:example-workload-cpu-"
    tortoises.autoscaling.mercari.com/container-based-memory-metric-prefix: "datadogmetric@example-prod:example-workload-memory-"
spec:
  metrics:
    - type: External
      external:
        metric:
          name: datadogmetric@example-prod:example-workload-cpu-app # CPU target of the container named "app"
        target:
          type: Value
          value: 60 # Tortoise regards this value as the target utilization for the CPU of the container named "app", and it will adjust this target value.
    - type: External
      external:
        metric:
          name: datadogmetric@example-prod:example-workload-cpu-istio-sidecar # CPU target of the container named "istio-sidecar"
        target:
          type: Value
          value: 80 # Tortoise regards this value as the target utilization for the CPU of the container named "istio-sidecar", and it will adjust this target value. 
    - type: External
      external:
        metric:
          name: datadogmetric@example-prod:example-workload-memory-app # memory target of the container named "app"
        target:
          type: Value
          value: 75 # Tortoise regards this value as the target utilization for the memory of the container named "app", and it will adjust this target value.
    - type: External
      external:
        metric:
          name: datadogmetric@example-prod:example-workload-memory-istio-sidecar # memory target of the container named "istio-sidecar"
        target:
          type: Value
          value: 70 # Tortoise regards this value as the target utilization for the memory of the container named "istio-sidecar", and it will adjust this target value.
```

### The container right sizing

Although it says "Horizontal", 
Tortoise rarely changes the container's size if it's too large or too small.

Because:
- if it's too large and each container's size is small, it's often better to increase each container's size.
- if it's too small and each container's size is big, it's often better to decrease each container's size.

The cluster admin can adjust the following as global configurations:
1. the minimum number of replicas: The Tortoise doesn't set lower value on HPA's `minReplicas`. And, when the num of replicas reaches this value, Tortoise temporary switches the scaling way to the Vertical until the container's size reaches `.spec.ResourcePolicy[*].MinAllocatedResources`
2. the preferred maximum number of replicas: When the num of replicas reaches this value, Tortoise temporary switches the scaling way to the Vertical until the container's size reaches "maximum resource amount" described below.
3. the maximum resource amount given to each resource: When the container's resource size reaches this value, Tortoise doesn't switch the scaling way to the Vertical.

(1) and (3) will be never broken by Tortoise. But, (2) is best effort basis and the num of replicas may actually be bigger.

I know it's complicated, describe specifically in the following examples.

#### Example1: reach the preferred maximum number of replicas

Let's say the global configurations are:
- the minimum number of replicas: 3
- the preferred maximum number of replicas: 10
- the maximum resource amount: 5cores

And, the target workload currently looks like:
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: 1cores
- resource request: CPU 2cores
- the num of replicas: 10 (the same as "the preferred maximum number of replicas")
- the resource consumption is increasing.

This case, Tortoise prefers not to increase the replica number more. 
Instead, Tortoise temporary switch the scaling way to Vertical to make each container bigger. 

After a while, the target workload looks like:
- resource request: 4cores. 
- the num of replicas: 10 (the same as "the preferred maximum number of replicas")
- the resource consumption starts to be decreasing.

Given the resource consumption starts to be decreasing, the Tortoise switch the scaling way back to Horizontal.
So, this workload will continue to work with 4 cores after this time.

If the traffic next day is very similar to this day, then Tortoise no longer needs to switch the scaling way to Vertical during peak time.

#### Example2: reach the preferred maximum number of replicas and the maximum resource amount

Let's say the global configurations are (the same as Example1):
- the minimum number of replicas: 3
- the preferred maximum number of replicas: 10
- the maximum resource: CPU 5cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1cores
- resource request: CPU 2cores
- the num of replicas: 10 (the same as "the preferred maximum number of replicas")
- the resource consumption is increasing.

As described in Example1, Tortoise temporary switches the scaling way to Vertical.

After a while, the target workload looks like:
- resource request: CPU 5cores (the same value as "the maximum resource amount given to each resource")
- the num of replicas: 10 (the same as "preferred maximum number of replicas")
- still the resource consumption is increasing.

The resource request reaches the "the maximum resource amount given to each resource" now.
So, Tortoise switch the scaling way back to the Horizontal.

After a while, the target workload looks like:
- resource request: CPU 5cores (CPU request is the same value as "the maximum resource amount")
- the num of replicas: 15 (more than "the preferred maximum number of replicas")
- the resource consumption starts to be decreasing.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during peak time
because it's already reached "the maximum resource amount".

#### Example 3: reach the minimum number of replicas

Let's say the global configurations are (the same as Example1):
- the minimum number of replicas: 3
- the preferred maximum number of replicas: 10
- the maximum resource amount given to each resource: CPU 5cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1cores
- resource request: CPU 2cores
- the num of replicas: 3 (the same as "the minumum number of replicas")
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1.5cores 
- the num of replicas: 3 (the same as "the minumum number of replicas")
- the resource consumption starts to be increasing.

Given the resource consumption starts to be increasing, the Tortoise switches the scaling way back to Horizontal.
So, this workload will continue to work with 1.5 cores after this time.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during off-peak time.

#### Example 4: reach the minimum number of replicas and `.spec.ResourcePolicy[*].MinAllocatedResources`

Let's say the global configurations are (the same as Example1):
- the minimum number of replicas: 3
- the preferred maximum number of replicas: 10
- the maximum resource amount given to each resource: CPU 5cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1cores
- resource request: CPU 2cores
- the num of replicas: 3 (the same as "the minimum number of replicas")
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1cores (the same as `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`)
- the num of replicas: 3 (the same as "the minimum number of replicas")
- still the resource consumption is decreasing.

The resource request reaches the `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`.
But, the num of replicas has already reached "the minumum number of replicas".

So, Tortoise won't change anything further.

### Sample

TODO: add sample tortoise.