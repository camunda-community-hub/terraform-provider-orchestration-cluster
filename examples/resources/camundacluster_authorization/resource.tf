resource "camundacluster_authorization" "example" {
  owner_type       = "GROUP"
  owner_id         = "my-group"
  resource_type    = "PROCESS_DEFINITION"
  resource_id      = "order-process"
  permission_types = ["READ_PROCESS_DEFINITION", "CREATE_PROCESS_INSTANCE"]
}

# A property-based authorization scopes permissions to resources by a named
# resource property instead of a specific resource ID. `resource_id` and
# `resource_property_name` are mutually exclusive.
# resource "camundacluster_authorization" "example_property_based" {
#   owner_type              = "GROUP"
#   owner_id                = "my-group"
#   resource_type           = "PROCESS_DEFINITION"
#   resource_property_name  = "customerId"
#   permission_types        = ["READ_PROCESS_DEFINITION"]
# }
