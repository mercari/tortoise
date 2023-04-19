## Contributor guide

<img alt="Tortoise" src="images/climbing.jpg" width="400px"/>

This document helps you to climb the road to the contributor.

### Debuggable Integration Test

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

(This debuggable integration test is inspired by [zoetrope/test-controller](https://github.com/zoetrope/test-controller) 
licensed under [MIT License](https://github.com/zoetrope/test-controller/blob/main/LICENSE))
