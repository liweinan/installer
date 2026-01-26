#!/bin/bash
# Debug script: Generate manifests and check zone consistency between CAPI and MAPI
# This script properly compares:
# - CAPI zones: from cluster-api/machines/10_inframachine_*-master-*.yaml (subnet filter)
# - MAPI zones: from openshift/99_openshift-machine-api_master-control-plane-machine-set.yaml

set -e

INSTALLER="${1:-./openshift-install}"
WORK_DIR="/tmp/debug-zone-check"
REGION="${2:-us-east-1}"

if [[ ! -x "$INSTALLER" ]]; then
    echo "Usage: $0 <openshift-install-path> [region]"
    exit 1
fi

rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

# Generate temporary SSH key
ssh-keygen -t ed25519 -f "$WORK_DIR/temp_key" -N "" -q

# Create install-config.yaml (without specifying zones - triggers the bug path)
cat > "$WORK_DIR/install-config.yaml" << EOF
apiVersion: v1
baseDomain: example.com
metadata:
  name: debug-test
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  replicas: 3
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  replicas: 3
platform:
  aws:
    region: ${REGION}
pullSecret: '{"auths":{"fake":{"auth":"fake"}}}'
sshKey: '$(cat "$WORK_DIR/temp_key.pub")'
EOF

echo "=========================================="
echo "OCPBUGS-69923 Zone Consistency Check"
echo "=========================================="
echo "Installer: $INSTALLER"
$INSTALLER version 2>/dev/null || true
echo ""
echo "Region: $REGION"
echo "=========================================="

# Generate manifests
echo ""
echo "Generating manifests..."
$INSTALLER create manifests --dir "$WORK_DIR" 2>&1 | head -20

echo ""
echo "=========================================="
echo "CAPI Zones (from cluster-api/machines/10_inframachine_*-master-*.yaml)"
echo "=========================================="
capi_zones=""
for file in $(find "$WORK_DIR"/cluster-api/machines -name "10_inframachine_*-master-*.yaml" -type f 2>/dev/null | sort); do
    # Extract zone from subnet filter name (e.g., "cluster-subnet-private-us-east-1a" -> "us-east-1a")
    subnet_name=$(yq eval '.spec.subnet.filters[0].values[0]' "$file" 2>/dev/null)
    # Extract zone from subnet name (last part after the last hyphen followed by zone pattern)
    zone=$(echo "$subnet_name" | grep -oE 'us-[a-z]+-[0-9][a-z]$' || echo "null")
    echo "  $(basename $file)"
    echo "    -> Subnet: $subnet_name"
    echo "    -> Zone: $zone"
    if [[ -n "$zone" && "$zone" != "null" ]]; then
        capi_zones="$capi_zones $zone"
    fi
done
capi_zones=$(echo "$capi_zones" | tr -s ' ' | sed 's/^ //;s/ $//')
echo ""
echo "CAPI Zones result: [$capi_zones]"

echo ""
echo "=========================================="
echo "MAPI Zones (from ControlPlaneMachineSet failureDomains)"
echo "=========================================="
mapi_zones=""
capi_count=$(echo "$capi_zones" | wc -w | tr -d ' ')
cpms_file="$WORK_DIR/openshift/99_openshift-machine-api_master-control-plane-machine-set.yaml"
if [[ -f "$cpms_file" ]]; then
    echo "  $(basename $cpms_file)"
    echo "  All failureDomains zones:"
    all_zones=$(yq eval '.spec.template.machines_v1beta1_machine_openshift_io.failureDomains.aws[].placement.availabilityZone' "$cpms_file" 2>/dev/null)
    idx=0
    for zone in $all_zones; do
        echo "    [$idx] $zone"
        if [[ "$zone" != "null" && -n "$zone" && $idx -lt $capi_count ]]; then
            mapi_zones="$mapi_zones $zone"
        fi
        idx=$((idx + 1))
    done
else
    echo "  ERROR: ControlPlaneMachineSet file not found!"
fi
mapi_zones=$(echo "$mapi_zones" | tr -s ' ' | sed 's/^ //;s/ $//')
echo ""
echo "MAPI Zones result (first $capi_count): [$mapi_zones]"

echo ""
echo "=========================================="
echo "Comparison Result"
echo "=========================================="
echo "CAPI: [$capi_zones]"
echo "MAPI: [$mapi_zones]"
echo ""

if [[ "$capi_zones" == "$mapi_zones" ]]; then
    echo "✅ CONSISTENT - Zones match"
    exit 0
else
    echo "❌ INCONSISTENT - Zone mismatch detected! (OCPBUGS-69923)"
    echo ""
    echo "This indicates the bug is present in this installer version."
    echo "The fix (PR #10188) adds slices.Sort() after UnsortedList() calls."
    exit 1
fi

echo ""
echo "=========================================="
echo "Manifests saved in: $WORK_DIR"
echo "=========================================="
