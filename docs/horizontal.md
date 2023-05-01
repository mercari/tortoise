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

#### Supported metrics in HPA

Currently, Tortoise supports:
- `type: Resource` metric if Pod has only one container.
- `type: ContainerResource` metric if Pod has only multiple containers.
- `type: External` metric if Pod has the annotations described below.

Regarding the `External`, given [the container resource metrics](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#container-resource-metrics) is still alpha as of v1.26, 
some companies are using the external metrics to fetch the container's resource utilization.

So, you can let Tortoise regard some external metrics as referring to the container's resource utilization.
You need to give the annotations to your HPA like:

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
a tortoise possibly changes the container's size if it's too large or too small.
- if the number of replicas equals `minimum-min-replicas`, make each container's size smaller instead of reducing the number of replicas.
- if the number of replicas equals `preferred-replicas-number-upper-limit`, make each container's size bigger instead of increasing the number of replicas.
  - But, when the resource request reaches `maximum-cpu-cores` or `maximum-memory-bytes`, tortoise will ignore `preferred-replicas-number-upper-limit`, and increase the number of replicas.

I know it's complicated, describe specifically in the following examples.

#### Example1: reach `preferred-replicas-number-upper-limit`

Let's say the global configurations are:
- `minimum-min-replicas` 3
- `preferred-replicas-number-upper-limit`: 10
- `maximum-cpu-cores`: 5 cores

And, the target workload currently looks like:
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: 1 cores
- resource request: CPU 2 cores
- the num of replicas: 10 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption is increasing.

This case, Tortoise prefers not to increase the replica number more. 
Instead, Tortoise temporary switch the scaling way to Vertical to make each container bigger. 

After a while, the target workload looks like:
- resource request: 4 cores. 
- the num of replicas: 10 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption starts to be decreasing.

Given the resource consumption starts to be decreasing, the Tortoise switch the scaling way back to Horizontal.
So, this workload will continue to work with 4 cores after this time.

If the traffic next day is very similar to this day, then Tortoise no longer needs to switch the scaling way to Vertical during peak time.

#### Example2: reach `preferred-replicas-number-upper-limit` and `maximum-cpu-cores` and `maximum-memory-bytes`

Let's say the global configurations are (the same as Example1):
- `minimum-min-replicas` 3
- `preferred-replicas-number-upper-limit`: 10
- `maximum-cpu-cores`: 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 10 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption is increasing.

As described in Example1, Tortoise temporary switches the scaling way to Vertical.

After a while, the target workload looks like:
- resource request: CPU 5 cores (the same value as `maximum-cpu-cores` and `maximum-memory-bytes` given to each resource")
- the num of replicas: 10 (the same as "preferred maximum number of replicas")
- still the resource consumption is increasing.

The resource request reaches the `maximum-cpu-cores` and `maximum-memory-bytes` given to each resource" now.
So, Tortoise switch the scaling way back to the Horizontal.

After a while, the target workload looks like:
- resource request: CPU 5 cores (CPU request is the same value as `maximum-cpu-cores` and `maximum-memory-bytes`)
- the num of replicas: 15 (more than `preferred-replicas-number-upper-limit`)
- the resource consumption starts to be decreasing.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during peak time
because it's already reached `maximum-cpu-cores` and `maximum-memory-bytes`.

#### Example 3: reach the minimum number of replicas

Let's say the global configurations are (the same as Example1):
- `minimum-min-replicas` 3
- `preferred-replicas-number-upper-limit`: 10
- `maximum-cpu-cores`: CPU 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 3 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1.5 cores 
- the num of replicas: 3 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption starts to be increasing.

Given the resource consumption starts to be increasing, the Tortoise switches the scaling way back to Horizontal.
So, this workload will continue to work with 1.5 cores after this time.

If the traffic next day is very similar to this day, 
Tortoise no longer needs to switch the scaling way to Vertical during off-peak time.

#### Example 4: reach the minimum number of replicas and `.spec.ResourcePolicy[*].MinAllocatedResources`

Let's say the global configurations are (the same as Example1):
- `minimum-min-replicas` 3
- `preferred-replicas-number-upper-limit`: 10
- `maximum-cpu-cores`: CPU 5 cores

And, the target workload currently looks like (the same as Example1):
- `.spec.ResourcePolicy[*].AutoscalingPolicy.CPU`: Horizontal
- `.spec.ResourcePolicy[*].MinAllocatedResources`: CPU 1 cores
- resource request: CPU 2 cores
- the num of replicas: 3 (the same as `preferred-replicas-number-upper-limit`)
- the resource consumption is decreasing.

This case, Tortoise prefers not to decrease the replica number more.
Instead, Tortoise temporary switch the scaling way to Vertical to make each container smaller.

After a while, the target workload looks like:
- resource request: CPU 1 cores (the same as `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`)
- the num of replicas: 3 (the same as `preferred-replicas-number-upper-limit`)
- still the resource consumption is decreasing.

The resource request reaches the `.spec.ResourcePolicy[*].MinAllocatedResources.CPU`.
But, the num of replicas has already reached `preferred-replicas-number-upper-limit`.

So, Tortoise won't change anything further.

### container size adjustment for the multiple container Pods

Although it says "Horizontal",
again, this optimization is kind of vertical change.

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
