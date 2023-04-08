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

## Documentations 

- [Concept](./docs/concept.md): describes a brief overview of tortoise.
- [Horizontal scaling](./docs/horizontal.md): describes how the Tortoise does the horizontal autoscaling.
- [Vertical scaling](./docs/vertical.md): describes how the Tortoise does the vertical autoscaling.
- [The emergency mode](./docs/emergency.md): describes the emergency mode.
- [Flag configurations for admin](./docs/flag-configuration.md): describes how the cluster admin can configure the global behavior via flags 
- [Technically details](./docs/internal.md): describes the technically details of Tortoise. (mostly for the contributors)

## API definition

- [Tortoise](./api/v1alpha1/tortoise_types.go)

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
