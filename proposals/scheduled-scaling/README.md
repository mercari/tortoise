# Scheduled Scaling

## Summary

<!--
This section is incredibly important for producing high-quality, user-focused
documentation such as release notes or a development roadmap. It should be
possible to collect this information before implementation begins, in order to
avoid requiring implementors to split their attention between writing release
notes and implementing the feature itself. 

A good summary is probably at least a paragraph in length.
-->

It proposes a new CRD `ScheduledScaling` to Tortoise ecosystem, which allows users to scale up Pods temporarily.

## Motivation

<!--
This section is for explicitly listing the motivation, goals, and non-goals of
this proposal.  Describe why the change is important and the benefits to users. The
motivation section can optionally provide links to [experience reports] to
demonstrate the interest in a proposal within the wider Kubernetes community.

[experience reports]: https://github.com/golang/go/wiki/ExperienceReports
-->

Tortoise is an animal looking at the past.
Under the shell, they record what happened in the past and operate autoscalers based on them.

On the other hands, we, human, is the only being that knows what happens in services in the future.
For example, if you plan to have a new-year campaign, starting 1/1,
and the traffic is supposed to get increased unusually, 
you probably want to pre-scale up your service so that you can enjoy your new year holidays
without being bothered by the alerts.

`ScheduledScaling` is a feature for the future.
It allows us to pre-scale up services before something actually happens. 

### Goals

<!--
List the specific goals of the proposal. What is it trying to achieve? How will we
know that this has succeeded?
-->

- Implement a feature to scale up services via Tortoise based on the schedule users define in `ScheduledScaling` CRD.

### Non-Goals

<!--
What is out of scope for this proposal? Listing non-goals helps to focus discussion
and make progress.
-->

- `ScheduledScaling` controller directly modifies HPAs or VPAs.
  - `ScheduledScaling` modifies `Tortoise` to let `Tortoise` controller do scaling.

## Proposal

<!--
This is where we get down to the specifics of what the proposal actually is.
This should have enough detail that reviewers can understand exactly what
you're proposing, but should not include things like API designs or
implementation. What is the desired outcome and how do we measure success?.
The "Design Details" section below is for the real
nitty-gritty.
-->

### User Stories (Optional)

<!--
Detail the things that people will be able to do if this proposal is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.
-->

#### Story 1

You plan to have a new-year campaign 1/1 - 1/5,
and the traffic is supposed to get increased unusually. 

So, you want to pre-scale up your service before the campaign starts. 

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: 2024-newyear-campaign
  namespace: mercari-awasome-jp 
spec:
  targetRefs:
    tortoiseName: cute-tortoise 
  strategy:
    static:
      # The replicas number of your service is usually 30 at peak time.
      # You expect the campaign makes the traffic 3 times bigger and want to scale out to 100 just in case.
      minReplicas: 100
      # Your memory is vertically scaled up by Tortoise and usually it's around 3GB.
      # You want to scale up it to 5GB, just in case.
      minAllocatedResources:
        - containerName: "app"
          resources: 
            memory: "5GB"
  schedule:
    startAt:   "2023-12-31T23:30:00Z" # Before the time the campaign starts.
    finishAt:  "2024-01-05T00:00:00Z" # When the campaign ends.
```

#### Story 2

Another team plans to launch a new microservice that calls your microservice on 4/1.
Your microservice will get a larger traffic, but it's difficult to calculate how much bigger the traffic will be.

So, you want to pre-scale up your service to be ready for an increase in the traffic before the launch of a new microservice. 
And, you will no longer need to forcibly scale up your service after the full release of a new microservice.

After `ScheduledScaling` expires, Tortoise gradually reduces the number of replicas and the resource requests.

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: new-ms-preparation
  namespace: mercari-awasome-jp 
spec:
  targetRefs:
    tortoiseName: cute-tortoise 
  strategy:
    static:
      # The replicas number of your service is usually 30 at peak time.
      # You expect the new microservice makes the traffic 1.5 times bigger and want to scale out to 50.
      minReplicas: 50
      # Your memory is vertically scaled up by Tortoise and usually it's around 3GB.
      # You want to scale up it to 5GB, just in case.
      minAllocatedResources:
        - containerName: "app"
          resources: 
            memory: "5GB"
  schedule:
    startAt:   "2024-03-31T23:30:00Z" # Before the time when the new microservice is launched.
    finishAt:  "2024-04-02T00:00:00Z" # Supposing the launch is completely finished within 1 day. The traffic from a new microservice should also become stable within 1 day.
```

### Notes/Constraints/Caveats (Optional)

<!--
What are the caveats to the proposal?
What are some important details that didn't come across above?
Go in to as much detail as necessary here.
This might be a good place to talk about core concepts and how they relate.
-->

### Risks and Mitigations

<!--
What are the risks of this proposal, and how do we mitigate? Think broadly.
-->

## Design Details

<!--
This section should contain enough information that the specifics of your
change are understandable. This may include API specs (though not always
required) or even code snippets. If there's any ambiguity about HOW your
proposal will be implemented, this is the place to discuss them.
-->

Propose creating a new CRD named `ScheduledScaling` as follows:

```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: 2024-newyear-campaign
  # namespaced resource. It should be created in the namespace that the target Tortoise exists.
  namespace: mercari-awasome-jp 
spec:
  targetRefs:
    # Target tortoise of scheduled scaling up. 
    tortoiseName: cute-tortoise 
  strategy:
    static:
      # minReplicas is the minimum replica number that Tortoise gives to HPA during this ScheduledScaling is valid.
      minReplicas: 100
      # minAllocatedResources is the minimum resource request that Tortoise gives to the Pods during this ScheduledScaling is valid.
      minAllocatedResources:
        - containerName: "app"
          resources: 
            memory: "5GB"
  schedule:
    # User can leave out this. If empty, it starts right after the creation. 
    startAt:   "2024-01-01T00:00:00Z" 
    # Must not empty. If empty, the validation webhook rejects the creation.
    finishAt:  "2024-01-05T00:00:00Z" 
```

In the future, we may want to create another strategy in `.spec.strategy` though,
this proposal only aims to create a simple `static` strategy.

### Future change in Tortoise

This `ScheduledScaling` allows us to decouple the pre-scaling feature from Tortoise.
Specifically, `.spec.resourcePolicy` can be completely removed from Tortoise in the future,
once this feature is fully available.

### Test Plan

<!--
**Note:** *Not required until targeted at a release.*
The goal is to ensure that we don't accept enhancements with inadequate testing.

All code is expected to have adequate tests (eventually with coverage
expectations). 
Please adhere to the [Kubernetes testing guidelines][testing-guidelines]
when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md
-->

##### Prerequisite testing updates

<!--
Based on reviewers feedback describe what additional tests need to be added prior
implementing this enhancement to ensure the enhancements have also solid foundations.
-->

n/a

##### Unit tests

<!--
In principle every added code should have complete unit test coverage, so providing
the exact set of tests will not bring additional value.
However, if complete unit test coverage is not possible, explain the reason of it
together with explanation why this is acceptable.
-->

All public functions in a new controller should be covered by unit tests.

##### Integration tests

<!--
Integration tests are contained in k8s.io/kubernetes/test/integration.
Integration tests allow control of the configuration parameters used to start the binaries under test.
This is different from e2e tests which do not allow configuration of parameters.
Doing this allows testing non-default options and multiple different and potentially conflicting command line options.
-->

All major scenarios should be covered by the integration tests.

### Feature Enablement and Rollback

###### How can this feature be rolled out in a live cluster?

<!--
Any pre-required action to roll out this feature?
-->

This feature requires a completely new controller with a new CRD.
Deploying them enables this feature.

###### Does enabling the feature change any default behavior?

<!--
Any change of default behavior may be surprising to users or break existing
automations, so be extremely careful here.
-->

No, `ScheduledScaling` is a opt-in feature and deploying this feature doesn't impact existing `Tortoise`.

###### Can the feature be disabled once it has been rolled out (i.e. can we roll back the enablement)?

<!--
Describe the consequences on existing workloads (e.g., if this is a runtime
feature, can it break the existing applications?).
-->

Yes, by removing a `ScheduledScaling` controller.

### Monitoring Requirements

###### How can an operator determine if the feature is in use by workloads?

<!--
Ideally, this should be a metric. Operations against the Kubernetes API (e.g.,
checking if there are objects with field X set) may be a last resort. Avoid
logs or events for this purpose.
-->

By querying `ScheduledScaling` resource.

###### How can someone using this feature know that it is working for their instance?

<!--
For instance, if this is a pod-related feature, it should be possible to determine if the feature is functioning properly
for each individual pod.
Pick one more of these and delete the rest.
Please describe all items visible to end users below with sufficient detail so that they can verify correct enablement
and operation of this feature.
Recall that end users cannot usually observe component logs or access metrics.
-->

- [x] Events
  - Event Reason: A new event `ScheduledScalingUp` (tentative name) will be implemented.

###### Are there any missing metrics that would be useful to have to improve observability of this feature?

<!--
Describe the metrics themselves and the reasons why they weren't added (e.g., cost,
implementation difficulties, etc.).
-->

A new controller should have a prenty of metrics that allow us to monitor things.

### Dependencies

###### Does this feature depend on any specific services running in the cluster?

<!--
Think about both cluster-level services (e.g. metrics-server) as well
as node-level agents (e.g. specific version of CRI). Focus on external or
optional services that are needed. For example, if this feature depends on
a cloud provider API, or upon an external software-defined storage or network
control plane.

For each of these, fill in the following—thinking about running existing user workloads
and creating new ones, as well as about cluster-level services (e.g. DNS):
  - [Dependency name]
    - Usage description:
      - Impact of its outage on the feature:
      - Impact of its degraded performance or high-error rates on the feature:
-->

It depends on `Tortoise` and thus tortoise controller.

## Drawbacks

<!--
Why should this proposal _not_ be implemented?
-->

n/a

## Alternatives

<!--
What other approaches did you consider, and why did you rule them out? These do
not need to be as detailed as the proposal, but should include enough
information to express the idea and why it was not acceptable.
-->

nothing.