# tortoise

<img alt="Tortoise" src="docs/images/tortoise_big.jpg" width="400px"/> 

Tortoise, they are living in the Kubernetes cluster. 

Tortoise, you need to feed only very few parameters to them.

Tortoise, they will soon start to eat historical usage data of Pods.

Tortoise, once you start to live with them, you no longer need to configure autoscaling by yourself.

## Install

Tortoise, you cannot get it from the breeder.

Tortoise, you need to get it from GitHub instead.

```shell
# Install CRDs into the K8s cluster specified in ~/.kube/config.
make install
# Deploy controller to the K8s cluster specified in ~/.kube/config.
make deploy
```

Tortoise, you don't need a rearing cage, but need VPA in your Kubernetes cluster before installing it.

## Usage

Tortoise, they only need the deployment name.

```yaml
apiVersion: autoscaling.mercari.com/v1beta2
kind: Tortoise
metadata:
  name: lovely-tortoise
  namespace: zoo
spec:
  updateMode: Auto
  targetRefs:
    scaleTargetRef:
      kind: Deployment
      name: sample
```

Tortoise, then they'll prepare/keep adjusting HPA and VPA to achieve efficient autoscaling based on the past behavior of the workload.

## Documentations 

- [Concept](./docs/concept.md): describes a brief overview of tortoise.
- [Horizontal scaling](./docs/horizontal.md): describes how the Tortoise does the horizontal autoscaling.
- [Vertical scaling](./docs/vertical.md): describes how the Tortoise does the vertical autoscaling.
- [Emergency mode](./docs/emergency.md): describes the emergency mode.
- [Configurations for admin](./docs/configuration.md): describes how the cluster admin can configure the global behavior via the configuration file. 
- [Technically details](./docs/internal.md): describes the technically details of Tortoise. (mostly for the contributors)
- [Contributor guide](./docs/contributor-guide.md): describes other stuff for the contributor. (testing etc)

## API definition

- [Tortoise](./api/v1beta2/tortoise_types.go)

## Contribution

Before implementing any feature changes as Pull Requests,
please raise the Issue and discuss what you propose with maintainers.

Also, please read the CLA carefully before submitting your contribution to Mercari. 
Under any circumstances, by submitting your contribution, 
you are deemed to accept and agree to be bound by the terms and conditions of the CLA.

https://www.mercari.com/cla/

## LICENSE

Copyright 2023 Mercari, Inc.

Licensed under the MIT License.
