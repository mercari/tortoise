## Configuration for admin

<img alt="Tortoise" src="images/eating.jpg" width="400px"/>

The cluster admin can set the global configurations via the configuration file.
The configuration file is passed via `--config` flag.

```
RangeOfMinMaxReplicasRecommendationHours:     The time (hours) range of minReplicas and maxReplicas recommendation (default: 1)
MinMaxReplicasRecommendationType:             The type of minReplicas and maxReplicas recommendation. The valid values are "daily" and "weekly" (default: weekly)
TTLHoursOfMinMaxReplicasRecommendation:       The TTL of minReplicas and maxReplicas recommendation (default: 720 (=30 days))
MaxReplicasFactor:                            The factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)
MinReplicasFactor:                            The factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)
ReplicaReductionFactor:                       The factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)
UpperTargetResourceUtilization:               The max target utilization that tortoise can give to the HPA (default: 90)
MinimumMinReplicas:                           The minimum minReplicas that tortoise can give to the HPA (default: 3)
PreferredReplicaNumUpperLimit:                The replica number which the tortoise tries to keep the replica number less than. As it says "preferred", the tortoise **tries** to keep the replicas number less than this, but the replica number may be more than this when other "required" rule will be violated by this limit. (default: 30)
MaximumCPUCores:                              The maximum CPU cores that the tortoise can give to the container (default: 10)
MaximumMemoryBytes:                           The maximum memory bytes that the tortoise can give to the container (default: 10Gi)
Timezone:                                     The timezone used to record time in tortoise objects (default: Asia/Tokyo)
TortoiseUpdateInterval:                       The interval of updating each tortoise (default: 15s)
```

### RangeOfMinMaxReplicasRecommendationHours

The time (hours) range of minReplicas and maxReplicas recommendation (default: 1)

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      minReplicas:
        - from: 0
          to: 1
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
        - from: 1
          to: 2
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
```

## MinMaxReplicasRecommendationType

The routine of minReplicas and maxReplicas recommendation. The valid values are "daily" and "weekly" (default: weekly)

#### "daily"

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      minReplicas:
        # This recommendation is from 0am to 1am on all days of week.
        - from: 0
          to: 1
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
        - from: 1
          to: 2
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
        # ...
        - from: 23
          to: 24
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
```

#### "weekly"

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      minReplicas:
        # This recommendation is from 0am to 1am on Sundays.
        - from: 0
          to: 1
          weekday: Sunday # Recommendation is generated for each day of week.
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
        - from: 1
          to: 2
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
        # ...
        - from: 23
          to: 24
          weekday: Saturday
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z
```

### TTLHoursOfMinMaxReplicasRecommendation

The TTL of minReplicas and maxReplicas recommendation (default: 720 (=30 days))

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      minReplicas:
        - from: 0
          to: 1
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 3
          updatedAt: 2023-01-01T00:00:00Z  # this recommendation will be expired on 2023-01-31.
```

### MaxReplicasFactor

The factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)

If the current replica number is 15 and `MaxReplicasFactor` is 2.0,
the maxReplicas recommendation from the current situation will be 30 (15 * 2.0).

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      maxReplicas:
        - from: 0
          to: 1
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 30 
          updatedAt: 2023-01-01T00:00:00Z  
```

### MinReplicasFactor

The factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)

If the current replica number is 10 and `MaxReplicasFactor` is 0.5,
the minReplicas recommendation from the current situation will be 5 (10 * 0.5).

```yaml
kind: Tortoise
#...
status: 
  recommendations:
    horizontal:
      minReplicas:
        - from: 0
          to: 1
          weekday: Sunday
          timezone: Asia/Tokyo
          value: 5
          updatedAt: 2023-01-01T00:00:00Z  
```

### ReplicaReductionFactor

The factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)

Let's say `ReplicaReductionFactor` is 0.95,
the minReplicas was increased to 100 due to the emergency mode,
and a user just turned off the emergency mode now.

Then, the `minReplicas` is going to change like:

100 --(*0.95)--> 95 --(*0.95)--> 91 -- ...

It's reduced every time tortoise is evaluated by the controller. (= once a `TortoiseUpdateInterval`)

### UpperTargetResourceUtilization

The max target utilization that tortoise can give to the HPA (default: 90)
So, HPA target utilization managed by tortoise won't be higher than this value.

### MinimumMinReplicas

The minimum minReplicas that tortoise can give to the HPA (default: 3)
So, HPA minReplicas managed by tortoise won't be smaller than this value.

### PreferredReplicaNumUpperLimit

The replica number which the tortoise tries to keep the replica number less than. (default: 30)

As it says "preferred", the tortoise **tries** to keep the replicas number less than this, 
but the replica number may be more than this when other "required" rules (`MaximumCPUCores` and `MaximumMemoryBytes`) will be violated by this limit. 

So, when the number of replicas reaches `PreferredReplicaNumUpperLimit`,
a tortoise will increase the Pod's resource request instead of increasing the number of replicas.

But, when the resource request reaches `MaximumCPUCores` or `MaximumMemoryBytes`,
a tortoise will ignore `PreferredReplicaNumUpperLimit`, and increase the number of replicas.

### MaximumCPUCores

The maximum CPU cores that the tortoise can give to the container (default: 10)

Note that it's the upper limit for the container, not for the Pod.

### MaximumMemoryBytes

The maximum memory bytes that the tortoise can give to the container (default: 10Gi)

Note that it's the upper limit for the container, not for the Pod.

### timezone

The timezone which used to record time in tortoise objects (default: Asia/Tokyo)

### TortoiseUpdateInterval 

The interval of updating each tortoise (default: 15s)

But, it may delay if there are many tortoise objects in the cluster.
