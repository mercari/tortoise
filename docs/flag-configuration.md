## Flag configurations for admin

<img alt="Tortoise" src="images/eating.jpg" width="400px"/>

The cluster admin can set the global configurations via flags.

```
--range-of-min-max-replicas-recommendation-hours: The time (hours) range of minReplicas and maxReplicas recommendation (default: 1)
--ttl-hours-of-min-max-replicas-recommendation:   The TTL of minReplicas and maxReplicas recommendation (default: 720 (=30 days))
--max-replicas-factor:                            The factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)
--min-replicas-factor:                            The factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)
--replica-reduction-factor:                       The factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)
--upper-target-resource-utilization:              The max target utilization that tortoise can give to the HPA (default: 90)
--minimum-min-replicas:                           The minimum minReplicas that tortoise can give to the HPA (default: 3)
--preferred-replicas-number-upper-limit:          The replica number which the tortoise tries to keep the replica number less than. As it says "preferred", the tortoise **tries** to keep the replicas number less than this, but the replica number may be more than this when other "required" rule will be violated by this limit. (default: 30)
--maximum-cpu-cores:                              The maximum CPU cores that the tortoise can give to the container (default: 10)
--maximum-memory-bytes:                           The maximum memory bytes that the tortoise can give to the container (default: 10Gi)
--timezone:                                       The timezone used to record time in tortoise objects (default: Asia/Tokyo)
--tortoise-update-interval:                       The interval of updating each tortoise (default: 15s)
```

### range-of-min-max-replicas-recommendation-hours

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

### ttl-hours-of-min-max-replicas-recommendation

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

### max-replicas-factor

The factor to calculate the maxReplicas recommendation from the current replica number (default: 2.0)

If the current replica number is 15 and `max-replicas-factor` is 2.0,
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

### min-replicas-factor

The factor to calculate the minReplicas recommendation from the current replica number (default: 0.5)

If the current replica number is 10 and `max-replicas-factor` is 0.5,
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

### replica-reduction-factor

The factor to reduce the minReplicas gradually after turning off Emergency mode (default: 0.95)

Let's say `replica-reduction-factor` is 0.95,
the minReplicas was increased to 100 due to the emergency mode,
and a user just turned off the emergency mode now.

Then, the `minReplicas` is going to change like:

100 --(*0.95)--> 95 --(*0.95)--> 91 -- ...

It's reduced every time tortoise is evaluated by the controller. (= once a `tortoise-update-interval`)

### upper-target-resource-utilization

The max target utilization that tortoise can give to the HPA (default: 90)
So, HPA target utilization managed by tortoise won't be higher than this value.

### minimum-min-replicas

The minimum minReplicas that tortoise can give to the HPA (default: 3)
So, HPA minReplicas managed by tortoise won't be smaller than this value.

### preferred-replicas-number-upper-limit

The replica number which the tortoise tries to keep the replica number less than. (default: 30)

As it says "preferred", the tortoise **tries** to keep the replicas number less than this, 
but the replica number may be more than this when other "required" rules (`maximum-cpu-cores` and `maximum-memory-bytes`) will be violated by this limit. 

So, when the number of replicas reaches `preferred-replicas-number-upper-limit`,
a tortoise will increase the Pod's resource request instead of increasing the number of replicas.

But, when the resource request reaches `maximum-cpu-cores` or `maximum-memory-bytes`,
a tortoise will ignore `preferred-replicas-number-upper-limit`, and increase the number of replicas.

### maximum-cpu-cores

The maximum CPU cores that the tortoise can give to the container (default: 10)

Note that it's the upper limit for the container, not for the Pod.

### maximum-memory-bytes

The maximum memory bytes that the tortoise can give to the container (default: 10Gi)

Note that it's the upper limit for the container, not for the Pod.

### timezone

The timezone which used to record time in tortoise objects (default: Asia/Tokyo)

### tortoise-update-interval 

The interval of updating each tortoise (default: 15s)

But, it may delay if there are many tortoise objects in the cluster.
