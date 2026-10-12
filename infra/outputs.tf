output "kube_context" {
  description = "kubectl context for the cluster."
  value       = local.kube_context
}

output "app_url" {
  description = "status-check API."
  value       = "http://localhost:${var.app_host_port}/api/v1/status"
}

output "argocd_url" {
  description = "Argo CD UI (user: admin)."
  value       = "http://localhost:${var.argocd_host_port}"
}

output "argocd_password_command" {
  description = "Prints the initial Argo CD admin password (bash / Git Bash)."
  value       = "kubectl --context ${local.kube_context} -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
}

output "registry" {
  description = "Push images here; the manifests reference the same name."
  value       = "localhost:${var.registry_port}"
}
