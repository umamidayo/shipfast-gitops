# --- Local image registry -----------------------------------------------------
# A plain Docker registry attached to the "kind" network. You push to
# localhost:<registry_port> from the host; the cluster pulls the same image name
# and containerd redirects it to the registry container over the Docker network.

resource "docker_image" "registry" {
  name         = "registry:2"
  keep_locally = true
}

resource "docker_container" "registry" {
  name    = local.registry_name
  image   = docker_image.registry.image_id
  restart = "unless-stopped"

  ports {
    internal = 5000
    external = var.registry_port
    ip       = "127.0.0.1"
  }

  # The "kind" network is created by kind along with the first cluster.
  networks_advanced {
    name = "kind"
  }

  depends_on = [kind_cluster.this]
}

# Tell containerd on the node that "localhost:<registry_port>" is served by the
# registry container. The script runs through `docker exec` as the interpreter,
# so it behaves the same from PowerShell, cmd, bash or zsh.
resource "terraform_data" "registry_hosts" {
  triggers_replace = [kind_cluster.this.id, docker_container.registry.id]

  provisioner "local-exec" {
    interpreter = ["docker", "exec", local.node_name, "sh", "-c"]
    command     = <<-SH
      dir=/etc/containerd/certs.d/localhost:${var.registry_port}
      mkdir -p "$dir"
      printf '[host."http://${local.registry_name}:5000"]\n' > "$dir/hosts.toml"
    SH
  }
}
