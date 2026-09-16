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

## Data sources

### `aoss_version`

Reads the software version reported by the switch.

```hcl
data "aoss_version" "this" {}
```
