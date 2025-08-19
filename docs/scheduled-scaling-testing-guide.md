# Scheduled Scaling Testing Guide

This guide provides comprehensive testing strategies for the new Scheduled Scaling functionality in Tortoise.

## 🎯 Overview

Scheduled Scaling allows you to define time-based scaling policies that automatically adjust resource constraints during specific time periods. This guide covers testing strategies from development to production.

## 🧪 Testing Levels

### 1. **Local Development Testing**

#### Prerequisites
- Go 1.23+
- kubectl configured
- Access to a Kubernetes cluster
- Tortoise controller deployed

#### Quick Start
```bash
# 1. Deploy the controller (if not already deployed)
make deploy

# 2. Run the automated test script
./scripts/test-scheduled-scaling.sh test-namespace

# 3. Monitor the behavior
./scripts/monitor-scheduled-scaling.sh test-namespace 10
```

### 2. **Manual Testing Scenarios**

#### Scenario 1: Basic Scheduled Scaling
```bash
# Create a simple scheduled scaling
kubectl apply -f test/scheduled-scaling-example.yaml

# Monitor the behavior
kubectl get scheduledscaling -w
kubectl get tortoise -w
kubectl get hpa -w
```

#### Scenario 2: Future Scheduled Scaling
```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: future-scaling
  namespace: default
spec:
  schedule:
    startAt: "2024-12-25T09:00:00Z"  # Future time
    finishAt: "2024-12-25T17:00:00Z"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minimumMinReplicas: 5
      minAllocatedResources:
        - containerName: "app"
          resource:
            cpu: "1000m"
            memory: "1Gi"
```

#### Scenario 3: Resource-Only Scaling
```yaml
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: resource-only-scaling
  namespace: default
spec:
  schedule:
    finishAt: "2024-12-25T23:59:59Z"
  targetRefs:
    tortoiseName: "my-tortoise"
  strategy:
    static:
      minAllocatedResources:
        - containerName: "app"
          resource:
            cpu: "2000m"
            memory: "2Gi"
```

### 3. **Integration Testing**

#### Test Cases to Verify

1. **Time Window Validation**
   - ✅ Scheduled scaling activates within the time window
   - ✅ Scheduled scaling deactivates after the time window
   - ✅ No effect outside the time window

2. **Replica Scaling**
   - ✅ HPA minReplicas is updated during active period
   - ✅ HPA minReplicas reverts after active period
   - ✅ Deployment replicas scale appropriately

3. **Resource Scaling**
   - ✅ Container resource requests are updated
   - ✅ Resource requests revert after active period
   - ✅ Multiple containers are handled correctly

4. **Error Handling**
   - ✅ Invalid time formats are rejected
   - ✅ Missing required fields are rejected
   - ✅ Non-existent tortoise references are handled

#### Verification Commands

```bash
# Check scheduled scaling status
kubectl get scheduledscaling -o yaml

# Check tortoise constraints
kubectl get tortoise <name> -o jsonpath='{.status.recommendations.constraints}'

# Check HPA settings
kubectl get hpa <name> -o jsonpath='{.spec.minReplicas}'

# Check deployment replicas
kubectl get deployment <name> -o jsonpath='{.spec.replicas}'

# Check controller logs
kubectl logs -n tortoise-system -l app=tortoise-controller -f

# Check events
kubectl get events --sort-by='.lastTimestamp' --field-selector involvedObject.kind=ScheduledScaling
```

### 4. **Production Testing**

#### Pre-Production Checklist
- [ ] Controller is deployed and healthy
- [ ] CRDs are installed
- [ ] RBAC permissions are configured
- [ ] Monitoring and alerting are set up

#### Gradual Rollout Strategy
1. **Phase 1**: Test with non-critical workloads
2. **Phase 2**: Test with staging environments
3. **Phase 3**: Test with production workloads during low-traffic periods
4. **Phase 4**: Full production deployment

#### Monitoring Metrics
```bash
# Check controller health
kubectl get pods -n tortoise-system

# Monitor resource usage
kubectl top pods -n tortoise-system

# Check for errors
kubectl logs -n tortoise-system -l app=tortoise-controller --tail=100 | grep -i error
```

## 🔍 Troubleshooting

### Common Issues

#### 1. Scheduled Scaling Not Activating
```bash
# Check if the controller is running
kubectl get pods -n tortoise-system

# Check controller logs
kubectl logs -n tortoise-system -l app=tortoise-controller

# Verify time format
kubectl get scheduledscaling <name> -o jsonpath='{.spec.schedule}'
```

#### 2. Changes Not Applied
```bash
# Check tortoise status
kubectl get tortoise <name> -o yaml

# Check HPA status
kubectl get hpa <name> -o yaml

# Check events
kubectl get events --field-selector involvedObject.name=<name>
```

#### 3. Validation Errors
```bash
# Check CRD validation
kubectl explain scheduledscaling.spec

# Validate YAML
kubectl apply -f your-file.yaml --dry-run=client
```

### Debug Commands

```bash
# Get detailed information about a scheduled scaling
kubectl describe scheduledscaling <name>

# Check all related resources
kubectl get all -l app=<app-label>

# Monitor real-time changes
kubectl get scheduledscaling,hpa,tortoise,deployment -w

# Check API server logs
kubectl logs -n kube-system kube-apiserver-<node> --tail=100
```

## 📊 Performance Testing

### Load Testing
```bash
# Create multiple scheduled scalings
for i in {1..10}; do
  kubectl apply -f - <<EOF
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: load-test-$i
  namespace: default
spec:
  schedule:
    finishAt: "$(date -u -d '+1 hour' '+%Y-%m-%dT%H:%M:%SZ')"
  targetRefs:
    tortoiseName: "test-tortoise"
  strategy:
    static:
      minimumMinReplicas: $((i + 1))
EOF
done
```

### Stress Testing
```bash
# Create rapid-fire scheduled scalings
for i in {1..50}; do
  kubectl apply -f - <<EOF
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: stress-test-$i
  namespace: default
spec:
  schedule:
    finishAt: "$(date -u -d '+5 minutes' '+%Y-%m-%dT%H:%M:%SZ')"
  targetRefs:
    tortoiseName: "test-tortoise"
  strategy:
    static:
      minimumMinReplicas: 1
EOF
  sleep 1
done
```

## 🚀 Advanced Testing

### Custom Test Scenarios

#### Business Hours Scaling
```yaml
# Scale up during business hours (9 AM - 5 PM)
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: business-hours-scaling
spec:
  schedule:
    startAt: "2024-12-25T09:00:00Z"
    finishAt: "2024-12-25T17:00:00Z"
  targetRefs:
    tortoiseName: "production-app"
  strategy:
    static:
      minimumMinReplicas: 5
      minAllocatedResources:
        - containerName: "app"
          resource:
            cpu: "1000m"
            memory: "1Gi"
```

#### Weekend Scaling
```yaml
# Scale down during weekends
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: weekend-scaling
spec:
  schedule:
    startAt: "2024-12-21T18:00:00Z"  # Friday 6 PM
    finishAt: "2024-12-23T09:00:00Z"  # Monday 9 AM
  targetRefs:
    tortoiseName: "production-app"
  strategy:
    static:
      minimumMinReplicas: 2
      minAllocatedResources:
        - containerName: "app"
          resource:
            cpu: "500m"
            memory: "512Mi"
```

## 📈 Monitoring and Alerting

### Key Metrics to Monitor
- Scheduled scaling activation/deactivation events
- Controller reconciliation frequency
- Error rates in controller logs
- Resource usage changes
- HPA scaling events

### Example Prometheus Queries
```promql
# Count of active scheduled scalings
count(scheduled_scaling_active{namespace="default"})

# Controller reconciliation rate
rate(tortoise_controller_reconcile_total[5m])

# Error rate
rate(tortoise_controller_errors_total[5m])
```

## 🎯 Success Criteria

A successful test should demonstrate:
- ✅ Scheduled scaling activates at the correct time
- ✅ Resource constraints are applied correctly
- ✅ Scaling reverts when the time window ends
- ✅ No impact on workloads outside the time window
- ✅ Controller handles errors gracefully
- ✅ Performance remains acceptable under load

## 📝 Test Report Template

After completing tests, document:
1. **Test Environment**: Cluster details, versions
2. **Test Scenarios**: What was tested
3. **Results**: Expected vs actual behavior
4. **Issues Found**: Any problems encountered
5. **Performance Metrics**: Resource usage, response times
6. **Recommendations**: Suggestions for improvement

---

For additional support, check the controller logs and events, or refer to the main Tortoise documentation.
