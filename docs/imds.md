# Instance metadata is denied to human users

## The problem

The box has an EC2 instance role, `bonnie-dev-instance`. It can read the
Secrets Manager entries the service needs: the Anthropic API key, the OIDC
client secret, the web signing secret.

Any process on the box can ask the instance metadata service for that role's
credentials, over plain HTTP, with no authentication beyond being able to reach
`169.254.169.254`. That includes processes belonging to ordinary users.

Measured on the deployed box before this was fixed, as the unprivileged account
`anthony-beckner`:

```
$ curl .../iam/security-credentials/
bonnie-dev-instance

$ aws secretsmanager get-secret-value --secret-id bonnie-dev/agent
{"TS_AGENT_API_KEY": "tfy_va_...
```

This defeats M3. Isolation between users is a permission bit — Bob's agent
reading Alice's home gets EACCES from the kernel. IMDS is not a file, so no
permission bit applies, and the kernel has no opinion about it. A user did not
even need to try: an agent following an instruction in a cloned repository
would do it on their behalf.

## The rule

`deploy/nftables/bonnie-imds.nft` rejects traffic to the metadata addresses
from any uid >= 1000:

```
ip  daddr 169.254.169.254 meta skuid >= 1000 reject
ip6 daddr fd00:ec2::254   meta skuid >= 1000 reject
```

The uid threshold is the whole design, and it works because of how accounts
are allocated here:

| account                  | uid   | IMDS  |
|--------------------------|-------|-------|
| root (web service, SSM)  | 0     | yes   |
| `bonnie` service account | 999   | yes   |
| `anthony-beckner`        | 1001  | no    |
| `bonnie-e2e`             | 1002  | no    |

Provisioned users get uids from 1000 up, and every system daemon sits below it.
So the service keeps reading its own secrets and the box keeps reporting
metrics, while no human — and no agent acting as one — inherits the role.

`reject` rather than `drop`: a rejected connection fails immediately with a
clear error, where a dropped one hangs until the SDK's timeout and looks like
a network fault. The failure should be legible.

The table is its own (`inet bonnie_imds`), so flushing or reloading it cannot
disturb any other ruleset.

## Ordering

`bonnie-imds-guard.service` is `Before=bonnie-web.service`. The web service
spawns agents as users, so if it started first there would be a window in which
an agent could reach metadata. `bonnie-deploy.sh` applies the guard before it
restarts the web service, for the same reason.

If the guard fails to start, both cloud-init and the deploy script warn loudly
rather than failing the rollout. A box with a working service and an open hole
is bad; a box that will not boot is worse, and the warning is visible in the
journal and the deploy output.

## What users do instead

They authenticate as themselves with IAM Identity Center — see the `aws-login`
skill. That gives each person their own credentials, their own permissions and
their own CloudTrail attribution, rather than a shared role that happens to be
over-privileged for what they are doing.

The skill tells the agent never to work around the block. A failure to reach
IMDS is the design functioning, not a fault to route around.

## Verifying it

As root, which should work:

```
curl -s -X PUT http://169.254.169.254/latest/api/token \
  -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' --max-time 3
```

As a user, which should be refused quickly:

```
sudo -u anthony-beckner curl -s --max-time 3 http://169.254.169.254/latest/meta-data/
```

And the service must still be able to read its own config, which is the thing
the uid threshold exists to preserve:

```
systemctl is-active bonnie-web.service
```
