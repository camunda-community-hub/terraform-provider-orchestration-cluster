terraform {
  required_providers {
    orchestration = {
      source = "registry.terraform.io/camunda-community-hub/orchestration-cluster"
    }
  }
}

variable "url" {}
variable "client_id" {}
variable "client_secret" {}
variable "login_url" {}
variable "audience" {}

provider "orchestration" {
  url = var.url

  #basic_auth = {
  #  username = "admin"
  #  password = "admin"
  #}

  #oidc = {
  #  login_url     = var.login_url
  #  audience      = var.audience
  #  client_id     = var.client_id
  #  client_secret = var.client_secret
  #}
}

#data "orchestration_cluster_user" "test" {
#  username = "admin"
#}
resource "orchestration_cluster_user" "test" {
  username = "xxx"
  name     = "Jonathan Ballet"
  email    = "jonathan.ballet@camunda.com"
  password = "test123"
}

data "orchestration_cluster_cluster_topology" "this" {}

output "topology" {
  value = data.orchestration_cluster_cluster_topology.this
}
