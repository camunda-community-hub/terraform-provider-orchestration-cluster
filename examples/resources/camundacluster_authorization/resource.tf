# ID-based authorization: grants a specific owner permissions on one concrete resource.
resource "camundacluster_authorization" "process_read" {
  owner_id      = "demo"
  owner_type    = "USER"
  resource_type = "PROCESS_DEFINITION"
  resource_id   = "order-process"

  permission_types = [
    "READ",
    "READ_PROCESS_DEFINITION",
  ]
}

# Property-based authorization: grants permissions on every resource matching a resource
# property, instead of a single resource ID. `resource_id` and `resource_property_name` are
# mutually exclusive; exactly one of the two must be set.
#
# resource "camundacluster_authorization" "process_read_by_property" {
#   owner_id                = "demo"
#   owner_type              = "USER"
#   resource_type           = "PROCESS_DEFINITION"
#   resource_property_name  = "*"
#
#   permission_types = [
#     "READ",
#   ]
# }
