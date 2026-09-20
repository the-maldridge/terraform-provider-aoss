# Terraform Provider for HP AOS-S Switches

This provider allows for idempotent configuration of switches running
the HP AOS-S operating system.  Switches in this line include the
popular (and cheap) HP2350 series gigabit Ethernet switches.

This switch works by initiating an SSH connection towards the switch
and remotely driving the config interface.  Technically AOS-S has an
API, but when I tried to get the documentation for this from HP's
Networking Support site I encountered broken auth walls, expired
certs, and broken HSTS pins (in that order).  I then saw a number of
reports on the forums that the API regularly gets broken by point
releases and has poor feature coverage.  Given those limitations,
driving the CLI seemed like the better approach, and would let me use
this provider as a proving ground before writing more complex
providers that use this same kind of "drive the CLI" workflow.

This provider would not have been possible without
[scrapligo](https://github.com/scrapli/scrapligo) which was the part
that I have been chewing on how to write for a few years, and was very
relieved to find someone else has already written.  If this provider
is useful to you, please support the scrapli team.

> [!NOTE]
>
> This provider was developed with extensive use of a local LLM
> cluster running Qwen3.8.  I started with an entire design for how
> this needed to fit together and then farmed out the shockingly
> tedious tasks of Terraform's type-system-in-a-type-system to a tool
> rather than my already headache saturated meat brain.

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

### `aoss_interface`

Manages a single physical port's `interface <n>` configuration block.
`interface` is the port number (1-255). `name` is the port name to assign
(empty for the default); `shutdown` (default `false`) administratively
shuts the port down. Removing the resource clears the port's name and
shutdown, leaving the port at its defaults.

```hcl
resource "aoss_interface" "uplink" {
  interface = 24
  name      = "OPTIMUX-TIE"
  shutdown  = false
}
```

### `aoss_management_vlan`

Sets the management VLAN (`management-vlan <id>` in configuration mode).
`vlan_id` must be between 1 and 4094.

```hcl
resource "aoss_management_vlan" "this" {
  vlan_id = 30
}
```

### `aoss_primary_vlan`

Sets the primary VLAN (`primary-vlan <id>` in configuration mode).
`vlan_id` must be between 1 and 4094.

```hcl
resource "aoss_primary_vlan" "this" {
  vlan_id = 1
}
```

### `aoss_trunk`

Creates a trunk circuit (`trunk <ports> <name> <mode>` in the root
configuration context). `ports` is a set of port numbers (1-255);
consecutive ports are collapsed into ranges in the emitted line. `name`
must be of the form `trk<N>` and `mode` must be `trunk` or `lacp`
(default `lacp`).

```hcl
resource "aoss_trunk" "uplink" {
  ports = [9, 10]
  name  = "trk1"
  mode  = "lacp"
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

### `aoss_interfaces`

Returns the list of all interfaces on the switch: every physical port and
every trunk circuit. Each entry has:

- `name` - the configured port name (empty string when a port has no
  configured name), or the trunk name (`trk<N>`) for logical interfaces.
- `number` - the port number for physical interfaces; `0` for logical (trunk)
  interfaces.
- `type` - one of `physical` (standalone port), `trunk-member` (a port
  enslaved to a trunk circuit), or `logical` (a trunk circuit).
- `shutdown` - whether the port is administratively shut down (the inverse
  of the `Port Enabled` state); always `false` for logical interfaces.
- `running` - whether the link is up (the `Link Status`); for logical
  interfaces, `true` when at least one member port is running.

```hcl
data "aoss_interfaces" "this" {}

output "interfaces" {
  value = data.aoss_interfaces.this.interfaces
}
```
