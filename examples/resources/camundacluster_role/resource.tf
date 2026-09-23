resource "camundacluster_role" "operations" {
  role_id     = "operations"
  name        = "Operations"
  description = "Operates the cluster's processes and incidents"
}
