terraform {
  required_providers {
    orchestration = {
      source = "camunda-community-hub/orchestration-cluster"
    }
  }
}

provider "orchestration" {
  url = "https://cluster.example.com/v2"

  # Optional: how long to wait for the eventually consistent read side (default 30s).
  consistency_timeout = "30s"

  # Optional: retries after a 5xx response, 0 disables (default 3).
  max_retries = 3

  oidc {
    login_url     = "https://login.example.com/oauth/token"
    audience      = "cluster.example.com"
    client_id     = var.client_id
    client_secret = var.client_secret
  }
}
