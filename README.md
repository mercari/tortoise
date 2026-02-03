# Tortoise


<img alt="Tortoise" src="docs/images/tortoise_big.jpg" width="400px"/> 

Get cute Tortoises into your Kubernetes garden and say goodbye to the days optimizing your rigid autoscalers. 

_Tortoise is still in the early stage and we recommend evaluating its behaviour with your services in your development environment carefully._

## Motivation

At Mercari, the responsibilities of the Platform team and the service development teams are clearly distinguished. Not all service owners possess expert knowledge of Kubernetes.

Also, Mercari has embraced a microservices architecture, currently managing over 1000 Deployments, each with its dedicated development team.

To effectively drive FinOps across such a sprawling landscape, 
it's clear that the platform team cannot individually optimize all services. 
As a result, they provide a plethora of tools and guidelines to simplify the process of the Kubernetes optimization for service owners.

But, even with them, manually optimizing various parameters across different resources, 
such as resource requests/limits, HPA parameters, and Golang runtime environment variables, presents a substantial challenge. 

Furthermore, this optimization demands engineering efforts from each team constantly - 
adjustments are necessary whenever there’s a change impacting a resource usage, which can occur frequently: 
Changes in implementation can alter resource consumption patterns, fluctuations in traffic volume are common, etc.

Therefore, to keep our Kubernetes clusters optimized, it would necessitate mandating all teams to perpetually engage in complex manual optimization processes indefinitely, 
or until Mercari goes out of business.

To address these challenges, the platform team has embarked on developing Tortoise, 
an automated solution designed to meet all Kubernetes resource optimization needs.

This approach shifts the optimization responsibility from service owners to the platform team (Tortoises), 
allowing for comprehensive tuning by the platform team to ensure all Tortoises in the cluster adapts to each workload. 
On the other hand, service owners are required to configure only a minimal number of parameters 
to initiate autoscaling with Tortoise, significantly simplifying their involvement.

See more details in the blog post:
- [Tortoise: Outpacing the Optimization Challenges in Kubernetes at Mercari](https://engineering.mercari.com/en/blog/entry/20240206-3a12bb1288/) 
- [人間によるKubernetesリソース最適化の”諦め”とそこに見るリクガメの可能性](https://engineering.mercari.com/blog/entry/20240206-3a12bb1288/)

## Install

You cannot get it from the breeder, you need to get it from GitHub instead.

```shell
# Install CRDs into the K8s cluster specified in ~/.kube/config.
make install
# Deploy controller to the K8s cluster specified in ~/.kube/config.
make deploy
```

You don't need a rearing cage, but need VPA in your Kubernetes cluster before installing it.

## Usage

As described in [Motivation](#motivation) section, Tortoise exposes many global parameters to a cluster admin, while it exposes few parameters in Tortoise resource.

### Cluster admin

See [Admin guide](./docs/admin-guide.md) to understand how to configure the tortoise controller to make it fit your workloads in one cluster.

### Tortoise users

Tortoise CRD itself has a very simple interface:

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

Then, Tortoise creates HPA and VPA under the hood. 
Despite its simple appearance, each tortoise stores a rich collection of historical data on resource utilization beneath its shell, 
and cleverly utilizes them to manage parameters in autoscalers. 

Please refer to [User guide](./docs/user-guide.md) to learn more about other parameters.

## Documentations 

- [User guide](./docs/user-guide.md): describes a minimum knowledge that the end-users have to know, 
and how they can configure Tortoise so that they can let tortoises autoscale their workloads.
- [Admin guide](./docs/admin-guide.md): describes how the cluster admin can configure the global behavior of tortoise. 
- [Global Disable Mode](./docs/global-disable-mode.md): describes how to use the global disable mode for testing scenarios.
- [Emergency mode](./docs/emergency.md): describes the emergency mode.
- [Horizontal scaling](./docs/horizontal.md): describes how the Tortoise does the horizontal autoscaling internally.
- [Vertical scaling](./docs/vertical.md): describes how the Tortoise does the vertical autoscaling internally.
- [Technically details](./docs/internal.md): describes the technically details of Tortoise. (mostly for the contributors)
- [Contributor guide](./docs/contributor-guide.md): describes other stuff for the contributor. (testing etc)

## API definition

- [Tortoise](./api/v1beta3/tortoise_types.go)

## Notes

Here's some notes that you may want to pay attention to before starting to use Tortoise.

- Tortoise only supports Deployment at the moment. In the future, [we'll support all resources supporting scale subresources](https://github.com/mercari/tortoise/issues/129).
- In Mercari, we've evaluated Tortoise with many Golang microservices, while there're a few services implemented in other languages using Tortoise. Any contributions would be welcome for enhance the recommendation for your language's services!

## Contribution

Before implementing any feature changes as Pull Requests,
please raise the Issue and discuss what you propose with maintainers.

A major change may have to be proposed via [proposal](./proposals/) process.

Also, please read the CLA carefully before submitting your contribution to Mercari. 
Under any circumstances, by submitting your contribution, 
you are deemed to accept and agree to be bound by the terms and conditions of the CLA.

https://www.mercari.com/cla/

## LICENSE

Copyright 2023 Mercari, Inc.

Licensed under the MIT License.
