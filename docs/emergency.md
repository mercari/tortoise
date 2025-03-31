## Emergency mode

<img alt="Tortoise" src="images/emergency.png" width="400px"/>

Emergency mode is one of the `UpdateMode`.

Autoscalers on tortoise are configured based on the historical resource usage.

If the workloads need to get scaled up in an unusual case (like unusual traffic increase etc),
you can turn on the emergency mode by setting `Emergency` on `.spec.UpdateMode` in Tortoise.

Note that Emergency mode is only available when your tortoise is `Working` or `PartlyWorking` AND has, at least, one Horizontal autoscaling policy.
You can check via `.status.tortoisePhase` and `.status.autoscalingPolicy` field:

```yaml
$ kubectl get tortoise your-tortoise -n your-namespace 

...
status:
  tortoisePhase: Working
  autoscalingPolicy:
  - containerName: application
    policy:
      cpu: Horizontal
      memory: Vertical
  - containerName: istio-proxy
    policy:
      cpu: Vertical
      memory: Vertical
```

### How emergency mode works

When emergency mode is enabled, tortoise increases the `minReplicas` of HPA to the same value as `maxReplicas`.

As described in [Horizontal scaling](./horizontal.md), `maxReplicas` gets changed to be fairly higher value every hour.
So, during emergency mode, the replicas will be kept fairly high value calculated from the past behavior for the safety.

### Turn off emergency mode 

Also, for the safety, after reverting `UpdateMode` from `Emergency` to `Auto`,
Tortoise tries to reduce the number of replicas to the original value gradually.
(A sudden decrease in a replica number is often dangerous.)

Specifically, the controller reduces `minReplicas` to the original value gradually by the following formula in one reconciliation:

```
{new minReplicas} = math.Trunc({the current minReplicas} * 0.95)
```

During gradually reducing the `minReplicas`, the Tortoise is in the `BackToNormal` state.

### Note

Emergency mode is only available for tortoises with `Running` or `BackToNormal` phase.
(because it requires enough historical data to work on)
