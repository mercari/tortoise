#!/bin/bash

# Monitoring script for Scheduled Scaling functionality
# Usage: ./scripts/monitor-scheduled-scaling.sh [namespace] [duration_minutes]

set -e

NAMESPACE=${1:-default}
DURATION=${2:-10}

echo "👀 Monitoring Scheduled Scaling functionality in namespace: $NAMESPACE"
echo "Duration: $DURATION minutes"
echo "=================================================="

# Function to get current time in RFC3339 format
get_rfc3339_time() {
    date -u '+%Y-%m-%dT%H:%M:%SZ'
}

# Function to check scheduled scaling resources
check_scheduled_scalings() {
    echo "📊 ScheduledScaling resources:"
    kubectl get scheduledscaling -n $NAMESPACE -o custom-columns="NAME:.metadata.name,PHASE:.status.scheduledScalingPhase,START:.spec.schedule.startAt,FINISH:.spec.schedule.finishAt" 2>/dev/null || echo "No ScheduledScaling resources found"
}

# Function to check tortoise constraints
check_tortoise_constraints() {
    echo "🐢 Tortoise constraints:"
    kubectl get tortoise -n $NAMESPACE -o custom-columns="NAME:.metadata.name,CONSTRAINTS:.status.recommendations.constraints.minimumMinReplicas" 2>/dev/null || echo "No Tortoise resources found"
}

# Function to check deployment replicas
check_deployment_replicas() {
    echo "📈 Deployment replicas:"
    kubectl get deployment -n $NAMESPACE -o custom-columns="NAME:.metadata.name,REPLICAS:.spec.replicas,READY:.status.readyReplicas" 2>/dev/null || echo "No Deployment resources found"
}

# Function to check HPA settings
check_hpa_settings() {
    echo "⚖️ HPA settings:"
    kubectl get hpa -n $NAMESPACE -o custom-columns="NAME:.metadata.name,MIN:.spec.minReplicas,MAX:.spec.maxReplicas,CURRENT:.status.currentReplicas" 2>/dev/null || echo "No HPA resources found"
}

# Function to check controller logs
check_controller_logs() {
    echo "📝 Recent controller logs (last 5 lines):"
    kubectl logs -n tortoise-system -l app=tortoise-controller --tail=5 2>/dev/null || echo "No controller logs found"
}

# Function to check events
check_events() {
    echo "📢 Recent events:"
    kubectl get events -n $NAMESPACE --sort-by='.lastTimestamp' --field-selector involvedObject.kind=ScheduledScaling -o custom-columns="TIME:.lastTimestamp,TYPE:.type,REASON:.reason,MESSAGE:.message" --tail=5 2>/dev/null || echo "No ScheduledScaling events found"
}

# Main monitoring loop
echo "Starting monitoring at $(get_rfc3339_time)"
echo "Will monitor for $DURATION minutes..."
echo ""

# Calculate end time
END_TIME=$(date -d "+$DURATION minutes" +%s)

while [ $(date +%s) -lt $END_TIME ]; do
    echo "🕐 $(get_rfc3339_time)"
    echo "----------------------------------------"
    
    check_scheduled_scalings
    echo ""
    
    check_tortoise_constraints
    echo ""
    
    check_deployment_replicas
    echo ""
    
    check_hpa_settings
    echo ""
    
    check_events
    echo ""
    
    check_controller_logs
    echo ""
    
    echo "⏳ Waiting 30 seconds..."
    echo "=================================================="
    sleep 30
done

echo "✅ Monitoring completed at $(get_rfc3339_time)"
