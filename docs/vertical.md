## Vertical scaling

<img alt="Tortoise" src="images/vertical1.jpg" width="300px"/> <img alt="Tortoise" src="images/vertical2.jpg" width="300px"/>

You can configure resource(s) to be scaled by horizontal scaling
by setting `Vertical` in `Spec.ResourcePolicy[*].AutoscalingPolicy`

When `Vertical` is specified on the resource,
that resource is basically updated based on the recommendation value from the VPA.

