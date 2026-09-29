## Tortoise CTL

<img alt="Tortoise" src="images/tortoise-hello.jpg" width="500px"/>

### Installation

```sh
go install github.com/mercari/tortoise/cmd/tortoisectl@latest
```

### `tortoisectl stop`

stop is the command to temporarily turn off tortoise(s) easily and safely.

It's intended to be used when your application is facing issues that might be caused by tortoise.
Specifically, it changes the tortoise updateMode to "Off" and restarts the scale target (Deployment or Argo Rollouts' Rollout) to bring the pods back to the original resource requests.
A Rollout is restarted via `.spec.restartAt`.

Also, with the `--no-lowering-resources` flag, it patches the scale target directly
so that changing tortoise to Off won't result in lowering the resource request(s), damaging the service.
e.g., if the Deployment declares 1 CPU request, and the current Pods' request is 2 CPU mutated by Tortoise,
it'd patch the deployment to 2 CPU request to prevent a possible negative impact on the service. 
Note that, for a Rollout, patching the Pod template creates a new revision and starts the canary/blue-green steps.
(If the Rollout refers to a Deployment via `.spec.workloadRef`, the referenced Deployment is patched.)

See full explanation by:

```sh
tortoisectl stop -h
```