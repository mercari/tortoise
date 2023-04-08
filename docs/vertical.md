## Vertical scaling

<img alt="Tortoise" src="images/vertical1.jpg" width="300px"/> <img alt="Tortoise" src="images/vertical2.jpg" width="300px"/>

You can configure resource(s) to be scaled by horizontal scaling
by setting `Vertical` in `Spec.ResourcePolicy[*].AutoscalingPolicy`

When `Vertical` is specified on the resource,
that resource is basically updated based on the recommendation value from the VPA.

### exceptional case; behave like Horizontal

Rarely the number of replicas get increased/decreased instead of increasing the resource request.
- When the resource request reaches `maximum-memory-bytes` or `maximum-cpu-cores`.
- When the resource usage gets increased unusually and the resource utilization is more than `upper-target-resource-utilization`.
