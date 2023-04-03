## Emergency mode

<img alt="Tortoise" src="images/emergency.png" width="400px"/>

Emergency mode is one of the `UpdateMode`.

Autoscalers on tortoise are configured based on the historical resource usage.

If the workloads need to get scaled up in an unusual case (like unusual traffic increase etc),
you can turn on the emergency mode by setting `Emergency` on `.spec.UpdateMode` in Tortoise.

Also, for the safety, after reverting `UpdateMode` from `Emergency` to `Auto`,
Tortoise tries to reduce the number of replicas to the original value gradually.
(A sudden decrease is mostly dangerous.)

### How emergency mode works

When emergency mode is enabled, tortoise increases the `minReplicas` to the same value as `maxReplicas`.

As described in [Horizontal scaling](./horizontal.md), `maxReplicas` gets changed to be fairly higher value every hour.
So, during emergency mode, the replicas will be kept fairly high value calculated from the past behavior for the safety.
