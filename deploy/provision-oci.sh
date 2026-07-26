#!/usr/bin/env bash
# Provisions the network and instance for the shop in OCI.
#
# Everything lives in its own compartment and its own VCN, with no peering to
# the VCN carrying hermes and the dev box. A compromise of the shop, which is
# the one thing here exposed to the internet, reaches nothing else.
#
# Infrastructure only. Installing the shop itself is deploy/README.md.
#
# Re-runnable: each step checks for what it would create and skips it.
set -euo pipefail

export OCI_CLI_AUTH=security_token

COMPARTMENT=${COMPARTMENT:-ocid1.compartment.oc1..aaaaaaaa3z36c7yep7ep2txi2l3po5hzthratg5zpedvivghhzl767erdgcq}
VCN=${VCN:-ocid1.vcn.oc1.iad.amaaaaaaxirugiqaavwwdmwdaoqtgyvaxpif6yjbvsf4gk4cujw7b6cdz7eq}

# Only AD-1 carries an E2.1.Micro allocation; AD-2 and AD-3 are limit 0, so an
# "out of capacity" here is worth retrying but not worth moving AD for.
AD=${AD:-gXAU:US-ASHBURN-AD-1}
SHAPE=${SHAPE:-VM.Standard.E2.1.Micro}
SUBNET_CIDR=${SUBNET_CIDR:-10.0.1.0/24}
ADMIN_KEY=${ADMIN_KEY:-$HOME/.ssh/dungeonbooks-shop-admin.pub}

say() { printf '\n== %s\n' "$1"; }

say "internet gateway"
IGW=$(oci network internet-gateway list -c "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[?"display-name"==`shop-igw`].id | [0]' --raw-output 2>/dev/null || true)
if [ -z "$IGW" ] || [ "$IGW" = "null" ]; then
  IGW=$(oci network internet-gateway create -c "$COMPARTMENT" --vcn-id "$VCN" \
    --is-enabled true --display-name shop-igw \
    --wait-for-state AVAILABLE --query 'data.id' --raw-output)
fi
echo "$IGW"

say "default route to the gateway"
RT=$(oci network vcn get --vcn-id "$VCN" --query 'data."default-route-table-id"' --raw-output)
oci network route-table update --rt-id "$RT" --force \
  --route-rules "[{\"destination\":\"0.0.0.0/0\",\"destinationType\":\"CIDR_BLOCK\",\"networkEntityId\":\"$IGW\"}]" \
  >/dev/null
echo "$RT"

say "public subnet"
SUBNET=$(oci network subnet list -c "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[?"display-name"==`shop-subnet`].id | [0]' --raw-output 2>/dev/null || true)
if [ -z "$SUBNET" ] || [ "$SUBNET" = "null" ]; then
  SUBNET=$(oci network subnet create -c "$COMPARTMENT" --vcn-id "$VCN" \
    --display-name shop-subnet --cidr-block "$SUBNET_CIDR" \
    --prohibit-public-ip-on-vnic false \
    --wait-for-state AVAILABLE --query 'data.id' --raw-output)
fi
echo "$SUBNET"

# The security list on the subnet stays as it is; the NSG is what actually
# admits traffic, so the rules live in one place rather than two.
say "network security group"
NSG=$(oci network nsg list -c "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[?"display-name"==`shop-nsg`].id | [0]' --raw-output 2>/dev/null || true)
if [ -z "$NSG" ] || [ "$NSG" = "null" ]; then
  NSG=$(oci network nsg create -c "$COMPARTMENT" --vcn-id "$VCN" \
    --display-name shop-nsg --wait-for-state AVAILABLE --query 'data.id' --raw-output)
fi
echo "$NSG"

say "ingress on 22 only"
# Port 22 is the whole product: visitors type no -p. Everything else stays shut.
oci network nsg rules add --nsg-id "$NSG" --security-rules '[
  {"direction":"INGRESS","protocol":"6","source":"0.0.0.0/0","sourceType":"CIDR_BLOCK",
   "description":"the shop",
   "tcpOptions":{"destinationPortRange":{"min":22,"max":22}}},
  {"direction":"INGRESS","protocol":"1","source":"0.0.0.0/0","sourceType":"CIDR_BLOCK",
   "description":"path MTU discovery, without which large writes hang",
   "icmpOptions":{"type":3,"code":4}}
]' >/dev/null 2>&1 || echo "(rules already present)"

oci network nsg rules add --nsg-id "$NSG" --security-rules '[
  {"direction":"EGRESS","protocol":"all","destination":"0.0.0.0/0","destinationType":"CIDR_BLOCK",
   "description":"Square API and package installs"}
]' >/dev/null 2>&1 || echo "(egress already present)"

say "ubuntu image for $SHAPE"
IMAGE=$(oci compute image list -c "$COMPARTMENT" \
  --operating-system "Canonical Ubuntu" --operating-system-version "24.04" \
  --shape "$SHAPE" --sort-by TIMECREATED --sort-order DESC \
  --query 'data[0].id' --raw-output)
echo "$IMAGE"

say "instance"
if [ ! -f "$ADMIN_KEY" ]; then
  echo "no admin public key at $ADMIN_KEY" >&2
  exit 1
fi
INSTANCE=$(oci compute instance list -c "$COMPARTMENT" \
  --query 'data[?"display-name"==`dungeonbooks-shop` && "lifecycle-state"!=`TERMINATED`].id | [0]' \
  --raw-output 2>/dev/null || true)
if [ -z "$INSTANCE" ] || [ "$INSTANCE" = "null" ]; then
  INSTANCE=$(oci compute instance launch -c "$COMPARTMENT" \
    --availability-domain "$AD" --shape "$SHAPE" --image-id "$IMAGE" \
    --display-name dungeonbooks-shop \
    --subnet-id "$SUBNET" --nsg-ids "[\"$NSG\"]" --assign-public-ip true \
    --ssh-authorized-keys-file "$ADMIN_KEY" \
    --wait-for-state RUNNING --query 'data.id' --raw-output)
fi
echo "$INSTANCE"

say "done"
oci compute instance list-vnics --instance-id "$INSTANCE" \
  --query 'data[].{public:"public-ip",private:"private-ip"}' --output table

cat <<'NEXT'

Next, in this order, or the box locks you out:

  1. ssh ubuntu@<public-ip>            (port 22, admin sshd still owns it)
  2. install tailscale, join the tailnet
  3. move admin sshd to the tailnet. Ubuntu 24.04 runs ssh.socket, so
     sshd_config's ListenAddress is ignored; set it on the socket instead:
       /etc/systemd/system/ssh.socket.d/tailnet-only.conf
       [Socket]
       ListenStream=
       ListenStream=<tailscale-ip>:22
     then: systemctl daemon-reload && systemctl restart ssh.socket
  4. reconnect over tailscale and CONFIRM before continuing
  5. only now is 22 free for the shop -- see deploy/README.md
     (the shop binds the VNIC private IP, not 0.0.0.0, or it collides
     with sshd on the tailnet address)

NEXT
