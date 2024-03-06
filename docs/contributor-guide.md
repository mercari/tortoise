## Contributor guide

<img alt="Tortoise" src="images/climbing.jpg" width="400px"/>

This document helps you in climbing the road to the contributor.

**Please discuss changes before editing this page, even minor ones.**

### Rules

- You **must** adhere to the linters configured in the repository.
Avoid using `//nolint` except in extraordinary circumstances. Strive for compliance even with minor issues.
- You **must** write unit tests for any new or modified functionality.
Almost all changes should be accompanied by corresponding tests to ensure behavior is as expected.
- Bugs always arise from insufficient or incorrect tests. 
You **must** update or add tests when fixing bugs to ensure the fix is valid. 
If you didn’t add a test, you didn’t fix the bug.
- **Do not** test anything manually in your Kubernetes cluster. 
Instead, you **must** implement all testing in the e2e tests.
- **Do not** bring any breaking change in Tortoise CRD. 
However, you **may** bring breaking changes in Golang functions or types within the repository - we're not developping the library and don't have to care much about downstream dependencies.

### Suggestion

- Enforce code-style issues with linters. When you want someone to fix a code-style, you can likely find a linter to detect such code-style issues in [golangci-lint](https://golangci-lint.run/usage/linters/).

#### Useful Golang official documents

Contributors are expected to adhere to these official guidelines.
Some of these recommendations are enforced by linters, while others are not.

- [Go Wiki: Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- [Go Wiki: Go Test Comments](https://go.dev/wiki/TestComments)
- [Effective Go](https://go.dev/doc/effective_go)

### Testing

The following command runs all tests (both unit tests and e2e tests) in the repository.

```shell
make test
```

#### E2e tests

Each Webhook and the controller have the integration tests.
- [/controllers/tortoise_controller_test.go](../controllers/tortoise_controller_test.go)
- [/api/autoscaling/v2/horizontalpodautoscaler_webhook_test.go](../api/autoscaling/v2/horizontalpodautoscaler_webhook_test.go)
- [/api/v1beta3/tortoise_webhook_test.go](../api/v1beta3/tortoise_webhook_test.go)

Their test suite are mostly defined in Yaml files to add test cases easier, e.g., [/controllers/testdata/](../controllers/testdata/).

Specifically about the controller's e2e test, it only simulates one reconciliation to simplify each test case.
For example, if you expect Tortoise is changed to state-a in one reconciliation and to state-b in next reconciliation,
you have to write two tests, one per reconciliation.

#### Auto-generate Integration Test Data(./controllers)

We have `make test-update` command to regenerate existing integration test data (./controllers/testdata/*/after/).

```shell
make test-update
```

When you implement some changes and it changes some test results, you can update test cases with it, without spending a waste time manually updating them.

But, make sure the generated test data is correct.

Also, note that a few test cases don't support this regeneration.

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
licensed under [MIT License](https://github.com/zoetrope/test-controller/blob/39902a6510642370973063afa7bcefe2997b7387/LICENSE) ❤️ )

### Linters

First, make sure you have a necessary tool in your local machine.

```shell
make dependencies
```

The following command runs all enabled linters.

```shell
make lint
```

Some linters can fix some problems automatically by the following command.

```shell
make lint-fix
```

See [.golangci.yml] about the linters enabled in the repository.
