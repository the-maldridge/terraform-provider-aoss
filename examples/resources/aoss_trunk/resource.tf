resource "aoss_trunk" "uplink" {
  ports = [9, 10]
  name  = "trk1"
  mode  = "lacp"
}
