# Global variable: GLOBAL/<name>
terraform import camundacluster_cluster_variable.global "GLOBAL/default-retries"

# Tenant-scoped variable: TENANT/<tenant_id>/<name>
terraform import camundacluster_cluster_variable.tenant "TENANT/my-tenant/greeting"
