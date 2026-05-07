terraform {
  required_providers {
    camundacluster = {
      source = "camunda.com/camunda/camunda-cluster"
    }
  }
}

variable "url" {}
variable "client_id" {}
variable "client_secret" {}
variable "login_url" {}
variable "audience" {}

provider "camundacluster" {
  url = var.url

  #basic_auth = {
  #  username = "admin"
  #  password = "admin"
  #}

  oidc = {
    login_url     = var.login_url
    audience      = var.audience
    client_id     = var.client_id
    client_secret = var.client_secret
  }
}

#data "camundacluster_user" "test" {
#  username = "admin"
#}
resource "camundacluster_user" "test" {
  username = "xxx"
  name     = "Jonathan Ballet"
  email    = "jonathan.ballet@camunda.com"
  password = "test123"
}

data "camundacluster_cluster_topology" "this" {}

output "topology" {
  value = data.camundacluster_cluster_topology.this
}
