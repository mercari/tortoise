## Technically details

<img alt="Tortoise" src="images/in-house.jpg" width="400px"/>

This document describes what tortoises are doing to update autoscalers and Pods in the house. 

This document is mostly for the contributors; Users don't necessarily need to know the things on this doc. 

### Which resource the tortoise changes exactly

The Tortoise controller doesn't touch the Pod resources. It doesn't touch the deployment resources as well.

It only updates the autoscalers; one HPA and two VPAs.

You can check which HPA and VPAs are on the tortoise via `.status.targets`. 

### Two VPAs - Updater and Monitor

The tortoise controller creates two VPAs per one Tortoise.

Two VPAs have different roles each other:
- Updater: it's used to update Pods via VPA's mutating webhook.
  - It's configured not to get updated by the VPA's recommender so that the tortoise controller can update the Pod resources request/limit by updating updater VPA's `status.recommendation.containerRecommendations`. 
- Monitor: it's used to read the recommendation value from the VPA recommender. 
  - It's configured to be dry-run so that it won't apply the recommendation by itself.

The tortoise controller uses the VPA's recommendation value in many places (Vertical autoscaling, calculating the best target utilization on HPA, etc),
and the recommendation value from the monitor VPA are used in such purposes.

And, whenever the tortoise controller wants to change the Pod's resource request,
it changes the updater VPA's `status.recommendation.containerRecommendations` instead of directly changing the resource defined in Pods.
