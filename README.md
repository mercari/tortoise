# Tortoise

<img alt="Tortoise" src="docs/images/tortoise_big.jpg" width="400px"/> 

Get a cute Tortoise into your Kubernetes garden and say goodbye to the days optimizing your rigid autoscalers. 

## Install

You cannot get it from the breeder, you need to get it from GitHub instead.

```shell
# Install CRDs into the K8s cluster specified in ~/.kube/config.
make install
# Deploy controller to the K8s cluster specified in ~/.kube/config.
make deploy
```

You don't need a rearing cage, but need VPA in your Kubernetes cluster before installing it.

## Motivation

Many developers are working in Mercari, and not all of them are the experts of Kubernetes. 
The platform has many tools and guides to simplify the task of optimizing resource requests, 
but it takes a lot of human effort because the situation around the applications gets changed very frequently and we have to keep optimizing them every time. 
(e.g., the implementation change could change the resource consumption, the amount of traffic could be changed, etc)

Also, there are another important component to be optimized for the optimization, which is HorizontalPodAutoscaler. 
It’s not a simple problem which we just set the target utilization as high as possible – 
there are many scenarios where the actual resource utilization doesn’t reach the target resource utilization 
(because of multiple containers, minReplicas, container’s size etc).

To reduce the human effort to keep optimizing the workloads, 
the platform team start to have Tortoise , which is designed to simplify the interface of autoscaling.

It aims to move the responsibility of optimizing the workloads from the application teams to tortoises. 
Application teams just need to set up Tortoise, and the platform team will never bother them again for the resource optimization - 
all actual optimization is done by Tortoise automatically. 

## Usage

Tortoise has a very simple interface:

```yaml
apiVersion: autoscaling.mercari.com/v1beta3
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

Yet, beneath its unassuming shell, lies a wealth of historical resource usage data, cunningly harnessed 
to deftly orchestrate HPA and VPA with finely-tuned parameters.

Please refer to [User guide](./docs/user-guide.md) for other parameters.

## Documentations 

- [User guide](./docs/user-guide.md): describes a minimum knowledge that the end-users have to know, 
and how they can configure Tortoise so that they can let tortoises autoscale their workloads.
- [Admin guide](./docs/admin-guide.md): describes how the cluster admin can configure the global behavior of tortoise. 
- [Emergency mode](./docs/emergency.md): describes the emergency mode.
- [Horizontal scaling](./docs/horizontal.md): describes how the Tortoise does the horizontal autoscaling internally.
- [Vertical scaling](./docs/vertical.md): describes how the Tortoise does the vertical autoscaling internally.
- [Technically details](./docs/internal.md): describes the technically details of Tortoise. (mostly for the contributors)
- [Contributor guide](./docs/contributor-guide.md): describes other stuff for the contributor. (testing etc)

## API definition

- [Tortoise](./api/v1beta3/tortoise_types.go)

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
