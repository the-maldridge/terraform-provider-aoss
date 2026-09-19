resource "aoss_vlan" "lan" {
  vlan_id  = 10
  name     = "LAN"
  untagged = [for i in range(1, 24) : tostring(i)]
  tagged   = ["24", "25", "26", "27", "28", "Trk1"]
}
