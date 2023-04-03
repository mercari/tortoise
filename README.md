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
# install all needed resources to the cluster.
make install
```

Tortoise, you don't need a rearing cage, but need VPA in your Kubernetes cluster before installing it.

## Documentations 

(Note that the feature described in the doc may not have been implemented yet.)

- [Concept](./docs/concept.md): describes a brief overview of tortoise.
- [Horizontal scaling](./docs/horizontal.md): describes how the Tortoise does the horizontal autoscaling.
- [Vertical scaling](./docs/vertical.md): describes how the Tortoise does the vertical autoscaling.
- [The emergency mode](./docs/emergency.md): describes about the emergency mode.

## API definition

- [Tortoise](./api/v1alpha1/tortoise_types.go)

## Contribution

We'll accept the contribution from outside of mercari in the future, but not at the moment.

## LICENSE

Copyright 2023 Mercari, Inc.

Licensed under the MIT License.
