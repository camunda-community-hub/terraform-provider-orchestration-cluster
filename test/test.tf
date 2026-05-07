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

  login_url     = var.login_url
  audience      = var.audience
  client_id     = var.client_id
  client_secret = var.client_secret
}

#data "camundacluster_user" "test" {
#  username = "admin"
#}
#resource "camundacluster_user" "test" {
#  username = "admin"
#  name     = "Jonathan Ballet"
#  email    = "jonathan.ballet@camunda.com"
#}

data "camundacluster_cluster_topology" "this" {}

output "topology" {
  value = data.camundacluster_cluster_topology.this
}
