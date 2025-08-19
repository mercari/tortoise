#!/bin/bash

# Test script for Scheduled Scaling functionality
# Usage: ./scripts/test-scheduled-scaling.sh [namespace]

set -e

NAMESPACE=${1:-default}
TEST_NAME="scheduled-scaling-test"

echo "🧪 Testing Scheduled Scaling functionality in namespace: $NAMESPACE"
echo "=================================================="

# Function to check if a resource exists
check_resource() {
    local resource_type=$1
    local resource_name=$2
    local namespace=$3
    
    if kubectl get $resource_type $resource_name -n $namespace >/dev/null 2>&1; then
        echo "✅ $resource_type/$resource_name exists"
        return 0
    else
        echo "❌ $resource_type/$resource_name not found"
        return 1
    fi
}

# Function to wait for resource to be ready
wait_for_resource() {
    local resource_type=$1
    local resource_name=$2
    local namespace=$3
    local timeout=${4:-60}
    
    echo "⏳ Waiting for $resource_type/$resource_name to be ready (timeout: ${timeout}s)..."
    
    for i in $(seq 1 $timeout); do
        if kubectl get $resource_type $resource_name -n $namespace -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q "True"; then
            echo "✅ $resource_type/$resource_name is ready"
            return 0
        fi
        sleep 1
    done
    
    echo "❌ $resource_type/$resource_name not ready after ${timeout}s"
    return 1
}

# Function to check scheduled scaling status
check_scheduled_scaling_status() {
    local name=$1
    local namespace=$2
    
    echo "📊 Checking ScheduledScaling status..."
    kubectl get scheduledscaling $name -n $namespace -o yaml | grep -A 10 "status:"
}

# Function to check tortoise constraints
check_tortoise_constraints() {
    local name=$1
    local namespace=$2
    
    echo "🐢 Checking Tortoise constraints..."
    kubectl get tortoise $name -n $namespace -o jsonpath='{.status.recommendations.constraints}' 2>/dev/null || echo "No constraints found"
}

# Function to check deployment replicas
check_deployment_replicas() {
    local name=$1
    local namespace=$2
    
    echo "📈 Checking deployment replicas..."
    kubectl get deployment $name -n $namespace -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "No replicas found"
}

# Function to check HPA settings
check_hpa_settings() {
    local name=$1
    local namespace=$2
    
    echo "⚖️ Checking HPA settings..."
    kubectl get hpa $name -n $namespace -o jsonpath='{.spec.minReplicas}' 2>/dev/null || echo "No minReplicas found"
}

# Create test namespace if it doesn't exist
if [ "$NAMESPACE" != "default" ]; then
    echo "📁 Creating namespace: $NAMESPACE"
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
fi

# Step 1: Deploy test resources
echo ""
echo "🚀 Step 1: Deploying test resources..."
kubectl apply -f test/scheduled-scaling-example.yaml -n $NAMESPACE

# Step 2: Wait for resources to be ready
echo ""
echo "⏳ Step 2: Waiting for resources to be ready..."

# Wait for deployment
wait_for_resource "deployment" "test-deployment" $NAMESPACE 120

# Wait for tortoise
wait_for_resource "tortoise" "test-tortoise" $NAMESPACE 120

# Wait for HPA
wait_for_resource "hpa" "test-hpa" $NAMESPACE 60

# Step 3: Verify initial state
echo ""
echo "🔍 Step 3: Verifying initial state..."

# Check initial deployment replicas
INITIAL_REPLICAS=$(kubectl get deployment test-deployment -n $NAMESPACE -o jsonpath='{.spec.replicas}')
echo "📊 Initial deployment replicas: $INITIAL_REPLICAS"

# Check initial HPA minReplicas
INITIAL_HPA_MIN=$(kubectl get hpa test-hpa -n $NAMESPACE -o jsonpath='{.spec.minReplicas}')
echo "⚖️ Initial HPA minReplicas: $INITIAL_HPA_MIN"

# Step 4: Create scheduled scaling
echo ""
echo "⏰ Step 4: Creating scheduled scaling..."

# Create a scheduled scaling that starts immediately and ends in 5 minutes
cat <<EOF | kubectl apply -f -
apiVersion: autoscaling.mercari.com/v1alpha1
kind: ScheduledScaling
metadata:
  name: $TEST_NAME
  namespace: $NAMESPACE
spec:
  schedule:
    finishAt: "$(date -u -d '+5 minutes' '+%Y-%m-%dT%H:%M:%SZ')"
  targetRefs:
    tortoiseName: "test-tortoise"
  strategy:
    static:
      minimumMinReplicas: 3
      minAllocatedResources:
        - containerName: "app"
          resource:
            cpu: "500m"
            memory: "512Mi"
EOF

# Step 5: Wait and monitor changes
echo ""
echo "👀 Step 5: Monitoring for changes (waiting 2 minutes)..."

for i in {1..12}; do
    echo ""
    echo "--- Check $i/12 ---"
    echo "Time: $(date)"
    
    # Check scheduled scaling status
    check_scheduled_scaling_status $TEST_NAME $NAMESPACE
    
    # Check tortoise constraints
    check_tortoise_constraints "test-tortoise" $NAMESPACE
    
    # Check deployment replicas
    check_deployment_replicas "test-deployment" $NAMESPACE
    
    # Check HPA settings
    check_hpa_settings "test-hpa" $NAMESPACE
    
    if [ $i -lt 12 ]; then
        echo "⏳ Waiting 10 seconds..."
        sleep 10
    fi
done

# Step 6: Verify final state
echo ""
echo "🔍 Step 6: Verifying final state..."

FINAL_REPLICAS=$(kubectl get deployment test-deployment -n $NAMESPACE -o jsonpath='{.spec.replicas}')
FINAL_HPA_MIN=$(kubectl get hpa test-hpa -n $NAMESPACE -o jsonpath='{.spec.minReplicas}')

echo "📊 Final deployment replicas: $FINAL_REPLICAS"
echo "⚖️ Final HPA minReplicas: $FINAL_HPA_MIN"

# Step 7: Cleanup
echo ""
echo "🧹 Step 7: Cleaning up test resources..."

kubectl delete scheduledscaling $TEST_NAME -n $NAMESPACE --ignore-not-found
kubectl delete -f test/scheduled-scaling-example.yaml -n $NAMESPACE --ignore-not-found

if [ "$NAMESPACE" != "default" ]; then
    echo "📁 Deleting namespace: $NAMESPACE"
    kubectl delete namespace $NAMESPACE --ignore-not-found
fi

echo ""
echo "✅ Testing completed!"
echo ""
echo "📋 Summary:"
echo "- Initial replicas: $INITIAL_REPLICAS"
echo "- Final replicas: $FINAL_REPLICAS"
echo "- Initial HPA min: $INITIAL_HPA_MIN"
echo "- Final HPA min: $FINAL_HPA_MIN"

if [ "$FINAL_REPLICAS" != "$INITIAL_REPLICAS" ] || [ "$FINAL_HPA_MIN" != "$INITIAL_HPA_MIN" ]; then
    echo "🎉 Scheduled scaling appears to be working!"
else
    echo "⚠️ No changes detected. Check the controller logs for issues."
fi
