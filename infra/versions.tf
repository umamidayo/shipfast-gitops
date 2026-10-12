terraform {
  required_version = ">= 1.8.0"

  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "~> 0.11"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 4.6"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.1"
    }
  }
}
