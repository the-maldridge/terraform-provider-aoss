data "aoss_interfaces" "this" {}

output "interfaces" {
  value = data.aoss_interfaces.this.interfaces
}
