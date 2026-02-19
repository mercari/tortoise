---
title: Custom HPA Scaling Limits with Tortoise
status: published
owner: kouzoh-platform-infra-jp
lastReviewed: 1970-01-01
type: guide
tags: [tortoise, hpa, scaling, custom]
---

# Custom HPA Scaling Limits with Tortoise

> Customize Tortoise HPA behavior based on your service requirements

This guide covers the advanced ServiceGroups feature in Tortoise, which allows platform administrators to configure different HPA scaling limits for different categories of services. This feature is designed for teams who need fine-grained control over their autoscaling behavior based on service characteristics.

## Overview

By default, Tortoise applies the same maximum replica limits to all services through the global `MaximumMaxReplicas` configuration, which is currently set to **100 replicas** in Citadel production. However, different types of services have different scaling requirements:

- **High-traffic services** may need to scale beyond 100 replicas during peak traffic
- **Cost-conscious services** might want lower limits to control costs
- **Critical services** might need different scaling behavior than experimental ones

The ServiceGroups feature allows you to request custom HPA scaling limits for your services based on your specific requirements.

## How It Works

ServiceGroups work through a configuration request process:

1. **Request Custom Limits**: Contact the platform team to request specific `maxReplicas` limits for your services
2. **Configuration**: Platform team configures your namespace and labels with the requested limits
3. **Automatic Application**: Tortoise automatically applies the custom limits to services matching your configuration

If your service doesn't match any custom ServiceGroup configuration, it uses the default limit of **100 replicas**.

## When to Request Custom Configuration

Consider requesting custom ServiceGroup configuration if your service falls into any of these categories or you need different limits than the default 100 replicas:

### Request Higher Limits (>100 replicas) if:
- Your service handles high user-facing traffic with significant spikes
- You need to scale beyond 100 replicas during peak periods
- Performance and availability are more critical than cost optimization
- You have validated that your service can effectively utilize 100+ replicas

### Request Lower Limits (<100 replicas) if:
- Cost control is a primary concern for your service
- Your service has predictable, steady traffic patterns
- Your service is batch-oriented or background processing
- You want to prevent accidental over-scaling and control resource costs

### Request Custom Limits for:
- Services with specific business requirements (e.g., 150, 200, or 50 replicas)
- Services that need limits based on downstream dependencies
- Services with regulatory or compliance constraints

## How to Request Custom Configuration

### Step 1: Contact Platform Team

To request custom scaling limits for your services, contact **#team-kouzoh-platform-infra-jp** team with the following information:

1. **Service Details**: Name and purpose of your service
2. **Namespace**: Which namespace your service is deployed in
3. **Labels**: Any specific labels your service uses (optional)
4. **Requested Limit**: The `maxReplicas` value you need (e.g., 150, 200, 50)
5. **Justification**: Business case for the custom limit

### Step 2: Configuration Applied

Once approved, the platform team will configure your namespace and labels with the custom limit.

## Configuration Examples

### Example 1: Requesting Higher Limits for High-Traffic Service

**Your Request to #team-kouzoh-platform-infra-jp**:
```
Hi team! I need custom scaling limits for our API gateway service:
- Service: api-gateway
- Namespace: production-api
- Current Issue: We're hitting the 100 replica limit during peak traffic
- Requested maxReplicas: 150
- Justification: Service handles user-facing traffic with 3x traffic spikes during events
```

**Your Service Configuration** (after platform team configures the namespace):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-gateway
  namespace: production-api  # Platform team configured this namespace for 150 replicas
  labels:
    app: api-gateway
    tier: frontend
spec:
  # ... your deployment spec
```

```yaml
# Your Tortoise configuration remains the same
apiVersion: autoscaling.mercari.com/v1beta3
kind: Tortoise
metadata:
  name: api-gateway-tortoise
  namespace: production-api
spec:
  updateMode: Auto
  targetRefs:
    scaleTargetRef:
      kind: Deployment
      name: api-gateway
```

**Result**: This Tortoise can now scale the HPA up to 150 replicas maximum.

### Example 2: Requesting Lower Limits for Cost Control

**Your Request to #team-kouzoh-platform-infra-jp**:
```
Hi team! I need custom scaling limits for our batch processing services:
- Services: All batch processors
- Namespace: batch-services
- Labels: tier=worker (optional, for more granular control)
- Requested maxReplicas: 50
- Justification: Cost optimization - these services have steady load and don't need >50 replicas
```

**Your Service Configuration** (after platform team configures the namespace):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: batch-processor
  namespace: batch-services  # Platform team configured this namespace for 50 replicas
  labels:
    app: batch-processor
    tier: worker
spec:
  # ... your deployment spec
```

**Result**: Services in this namespace are limited to 50 replicas maximum, helping control costs.

### Example 3: Default Behavior (No Custom Configuration)

Services that don't have custom configuration will use the default limit:

```yaml
apiVersion: apps/v1 
kind: Deployment
metadata:
  name: experimental-service
  namespace: development  # No custom configuration for this namespace
  labels:
    app: experimental-service
spec:
  # ... your deployment spec
```

**Result**: This Tortoise will use the default limit of **100 replicas**.

## Understanding Service Group Matching

Once the platform team configures your custom limits, the matching works as follows:

### Namespace-Based Matching
Services are primarily matched based on their namespace. For example, if you requested:
- `production-api` namespace → 150 replicas limit
- `batch-services` namespace → 50 replicas limit

### Label-Based Matching (Advanced)
For more granular control, the platform team can also configure label selectors:

```yaml
# Example: Services with specific labels get custom limits
metadata:
  namespace: my-services
  labels:
    tier: worker          # Only services with this label get the custom limit
    cost-tier: optimized  # Additional label selector for more precision
```

This allows different services within the same namespace to have different limits if needed.

## Monitoring Your Service Group Assignment

### Check Your Tortoise Status
You can verify which limits are being applied to your service by checking your Tortoise status:

```bash
kubectl describe tortoise <your-tortoise-name> -n <your-namespace>
```

Look for events or status information that indicates the maximum replica limits being applied.

### Metrics and Observability
Monitor these metrics to understand your service's scaling behavior:

- `mercari.tortoise.proposed_hpa_maxreplicas` - The maximum replicas Tortoise recommends
- `mercari.tortoise.current_hpa_maxreplicas` - The current HPA maximum replicas setting

You can also use the Tortoise dashboard to visualize how service groups affect your scaling:
https://app.datadoghq.com/dashboard/h4s-vry-dyv/tortoise-user-dashboard

## Best Practices

### Before Requesting Custom Limits

**Analyze Your Current Usage:**
1. **Monitor First**: Use Off mode Tortoise to see what limits would be recommended
2. **Check Current Peaks**: Review your service's historical scaling patterns
3. **Validate Need**: Ensure your service can actually benefit from custom limits
4. **Cost Impact**: Consider the cost implications of higher limits

**Prepare Your Request:**
1. **Gather Data**: Collect metrics showing why you need custom limits
2. **Business Justification**: Explain the business impact of the current limits
3. **Specific Requirements**: Request exact numbers rather than ranges
4. **Timeline**: Mention if this is urgent or can be planned

### After Custom Configuration

**Gradual Adoption:**
1. **Test in Non-Production**: Start with development or staging environments
2. **Monitor Closely**: Watch for unexpected scaling behavior
3. **Cost Monitoring**: Keep track of resource costs after configuration changes
4. **Validate Effectiveness**: Ensure the custom limits solve your original problem

## Troubleshooting

### My Service Isn't Using the Expected Custom Limits

1. **Verify Configuration**: Check if the platform team has confirmed your configuration is applied
2. **Check Request Status**: Follow up with #team-kouzoh-platform-infra-jp if your request is still pending
3. **Namespace/Labels**: Verify your service is deployed in the configured namespace with correct labels
4. **Tortoise Restart**: Sometimes restarting the Tortoise controller helps pick up new configuration

### My Request Was Rejected or Modified

If the platform team suggests different limits than requested:

1. **Understand Reasoning**: Ask for explanation of the decision
2. **Provide Additional Context**: Share more data about your scaling needs
3. **Compromise**: Consider accepting modified limits and requesting adjustments later based on real usage

## Common Questions

### Can I Override Custom Limits?

Custom limits are enforced at the platform level and cannot be overridden by individual Tortoise configurations. This ensures consistent resource management across the platform.

### How Do I Know What Custom Limits Are Applied to My Service?

1. **Check with Platform Team**: Ask #team-kouzoh-platform-infra-jp about your namespace configuration
2. **Monitor Tortoise Behavior**: Observe the maximum replicas Tortoise applies to your HPA
3. **Review Tortoise Events**: Check kubectl events for information about applied limits

### What Happens If I Hit My Custom Limits During Traffic Spikes?

Services will scale up to their configured custom limit. If you consistently hit your limits and need more capacity:

1. **Gather Evidence**: Collect metrics showing the need for higher limits
2. **Request Increase**: Contact #team-kouzoh-platform-infra-jp with data supporting the need for higher limits
3. **Temporary Relief**: Discuss emergency procedures for critical traffic events

### How Long Does It Take to Get Custom Configuration?

- **Simple Requests**: Usually processed within 1-2 business days
- **Complex Requests**: May take longer if they require additional review or testing
- **Urgent Requests**: Contact #team-kouzoh-platform-infra-jp directly for expedited processing

### Can I Request Different Limits for Different Services in the Same Namespace?

Yes! The platform team can configure label-based selectors to give different services in the same namespace different limits. Include this requirement in your request.

## Getting Help

If you have questions about custom scaling limits or need assistance with configuration:

- **Slack**: #team-kouzoh-platform-infra-jp
- **For Custom Limit Requests**: Contact #team-kouzoh-platform-infra-jp with your requirements
- **For Configuration Issues**: Reach out with your specific service details and observed behavior
- **For Urgent Issues**: Tag the platform team directly in #team-kouzoh-platform-infra-jp

## Related Documentation

- [Tortoise Main Guide](./tortoise.md) - Basic Tortoise usage and configuration
- [Kubernetes Kit: Scaling](../guides/k8s-kit/scaling.md) - Integration with Kubernetes Kit
- [HPA Configuration](../guides/how-to-create-hpa.md) - Manual HPA configuration (if needed) 