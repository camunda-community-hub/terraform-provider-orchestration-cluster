provider "camundacluster" {
  url = "https://cluster.example.com/v2"

  oidc {
    login_url     = "https://login.example.com/oauth/token"
    audience      = "cluster.example.com"
    client_id     = var.client_id
    client_secret = var.client_secret
  }
}
