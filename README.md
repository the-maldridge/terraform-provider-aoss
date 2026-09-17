# Terraform Provider for HP AOS-S Switches

This provider allows for idempotent configuration of switches running the HP
AOS-S operating system.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform) >= 1.0
- Go >= 1.26 (for development)

## Provider configuration

```hcl
provider "aoss" {
  host     = var.switch_host
  username = var.switch_user
  password = var.switch_pass
}
```

The connection values may also be supplied via the `AOSS_HOST`,
`AOSS_USERNAME`, and `AOSS_PASSWORD` environment variables.

## Resources

### `aoss_hostname`

Sets the switch hostname (`hostname <name>` in configuration mode).

```hcl
resource "aoss_hostname" "this" {
  hostname = "idf02"
}
```

### `aoss_vlan`

Manages a VLAN and its member ports and trunk circuits. `tagged` and
`untagged` are sets of member references, each either a port number or a
trunk circuit name of the form `Trk<N>` (the `trk` prefix is matched
case-insensitively; state is stored in the canonical `Trk<N>` spelling).
`dhcp` (default `false`) controls whether the VLAN interface obtains its IP
address via DHCP.

```hcl
resource "aoss_vlan" "lan" {
  vlan_id  = 10
  name     = "LAN"
  untagged = [for i in range(1, 24) : tostring(i)]
  tagged   = ["24", "25", "26", "27", "28", "Trk1"]
}
```

## Data sources

### `aoss_version`

Reads the software version reported by the switch.

```hcl
data "aoss_version" "this" {}
```
