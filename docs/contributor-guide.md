## Contributor guide

<img alt="Tortoise" src="images/climbing.jpg" width="400px"/>

This document helps you to climb the road to the contributor.

### Coding guide

You **must** follow the linter configured in the repository. 
Avoid using `//nolint` as much as possible even if you just want to ignore very tiny thing. 
(broken windows theory)

You also can run the following command and the linters fix some problems automatically.

```
make lint-fix
```

We only configure a few linters now, 
if you want to enforce something further, please open the issue and discuss.

### Testing

You **must** implement the unit test (or add the test cases to existing tests)
when you change something.

Almost all behavior changes likely have to have an addition/change in testing.

#### The integration tests

Each Webhook and the controller have the integration tests.
- [/controllers/tortoise_controller_test.go](../controllers/tortoise_controller_test.go)
- [/api/autoscaling/v2/horizontalpodautoscaler_webhook_test.go](../api/autoscaling/v2/horizontalpodautoscaler_webhook_test.go)
- [/api/v1beta3/tortoise_webhook_test.go](../api/v1beta3/tortoise_webhook_test.go)

If you implement a major feature, that is something achieved by combining multiple services,
or you implement something in webhooks, you **must** add a new test case in them.

#### Debuggable Integration Test

We have `make test-debug` command to help you with debugging of the failing integration test.

```shell
make test-debug
```

If an integration test is failed, you'll see this message, and you can follow it to communicate with kube-apiserver to investigate the failure. 

```
You can use the following command to investigate the failure:
$ kubectl --kubeconfig=/var/folders/3r/38tmpwm105d59kp_zlfbch0h0000gp/T/k8s_test_framework_1274319527/4210920706.kubecfg
When you have finished investigation, clean up with the following commands:
$ pkill kube-apiserver
$ pkill etcd
$ rm -rf /var/folders/3r/38tmpwm105d59kp_zlfbch0h0000gp/T/k8s_test_framework_1274319527
```

(This debuggable integration test is inspired by [zoetrope/test-controller](https://github.com/zoetrope/test-controller/tree/39902a6510642370973063afa7bcefe2997b7387) 
licensed under [MIT License](https://github.com/zoetrope/test-controller/blob/39902a6510642370973063afa7bcefe2997b7387/LICENSE))
