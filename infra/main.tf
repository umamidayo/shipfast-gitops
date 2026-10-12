locals {
  # The home directory starts with "/" on Linux/macOS and a drive letter on Windows.
  is_windows  = substr(pathexpand("~"), 0, 1) != "/"
  docker_host = coalesce(var.docker_host, local.is_windows ? "npipe:////./pipe/docker_engine" : "unix:///var/run/docker.sock")

  kube_context  = "kind-${var.cluster_name}"
  node_name     = "${var.cluster_name}-control-plane"
  registry_name = "${var.cluster_name}-registry"

  # Node ports that kind maps to the host. These must match
  # deploy/components/nodeport and the Argo CD service values in argocd.tf.
  app_node_port    = 30080
  argocd_node_port = 30081
}

provider "docker" {
  host = local.docker_host
}

provider "helm" {
  kubernetes = {
    host                   = kind_cluster.this.endpoint
    client_certificate     = kind_cluster.this.client_certificate
    client_key             = kind_cluster.this.client_key
    cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  }
}

# --- Kind cluster -------------------------------------------------------------
# A single-node Kubernetes cluster running as a Docker container. Its kubeconfig
# is merged into ~/.kube/config under the context "kind-<cluster_name>".

resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = var.node_image
  wait_for_ready = true
  # Merge into the default kubeconfig (kind's own behaviour) instead of the
  # provider's default of writing ./<name>-config next to the .tf files.
  kubeconfig_path = pathexpand("~/.kube/config")

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    # Let containerd read per-registry config from /etc/containerd/certs.d
    # (populated by terraform_data.registry_hosts in registry.tf).
    containerd_config_patches = [
      <<-TOML
      [plugins."io.containerd.grpc.v1.cri".registry]
        config_path = "/etc/containerd/certs.d"
      TOML
    ]

    node {
      role = "control-plane"

      extra_port_mappings {
        container_port = local.app_node_port
        host_port      = var.app_host_port
        listen_address = "127.0.0.1"
        protocol       = "TCP"
      }

      extra_port_mappings {
        container_port = local.argocd_node_port
        host_port      = var.argocd_host_port
        listen_address = "127.0.0.1"
        protocol       = "TCP"
      }
    }
  }
}
