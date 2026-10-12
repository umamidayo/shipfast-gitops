# --- Git source (what Argo CD watches) ----------------------------------------

variable "git_repo_url" {
  description = "Git URL of this repository, as Argo CD should clone it (e.g. https://github.com/you/shipfast-gitops.git)."
  type        = string

  validation {
    condition     = can(regex("^(https://|git@|ssh://)", var.git_repo_url))
    error_message = "git_repo_url must be an https:// or SSH git URL that the cluster can reach."
  }
}

variable "git_revision" {
  description = "Branch, tag or commit Argo CD tracks."
  type        = string
  default     = "main"
}

variable "git_username" {
  description = "Username for a private repo (any non-empty value works with a GitHub token)."
  type        = string
  default     = "git"
}

variable "git_token" {
  description = "Read-only access token for a private repo. Leave empty for a public repo."
  type        = string
  default     = ""
  sensitive   = true
}

variable "app_path" {
  description = "Path in the repo of the Kustomize overlay Argo CD deploys."
  type        = string
  default     = "deploy/overlays/local"
}

variable "app_namespace" {
  description = "Namespace the status-check app is deployed into."
  type        = string
  default     = "status-check"
}

# --- Cluster ------------------------------------------------------------------

variable "cluster_name" {
  description = "Name of the Kind cluster (the kubectl context becomes kind-<name>)."
  type        = string
  default     = "shipfast"
}

variable "node_image" {
  description = "kindest/node image, which pins the Kubernetes version. null uses the provider's default."
  type        = string
  default     = null
}

variable "app_host_port" {
  description = "Host port that serves the status-check API (node port 30080)."
  type        = number
  default     = 8080
}

variable "argocd_host_port" {
  description = "Host port that serves the Argo CD UI (node port 30081)."
  type        = number
  default     = 8081
}

# --- Local image registry -----------------------------------------------------

variable "registry_port" {
  description = "Host port of the local registry. Images are pushed to localhost:<port>/..."
  type        = number
  default     = 5001
}

variable "docker_host" {
  description = "Docker daemon address. null picks the platform default (named pipe on Windows, unix socket elsewhere)."
  type        = string
  default     = null
}

# --- Argo CD ------------------------------------------------------------------

variable "argocd_chart_version" {
  description = "argo-cd Helm chart version."
  type        = string
  default     = "10.10.2"
}

variable "argocd_apps_chart_version" {
  description = "argocd-apps Helm chart version."
  type        = string
  default     = "2.0.6"
}

variable "argocd_poll_interval" {
  description = "How often Argo CD polls Git for changes. Lower = faster deploys, more Git traffic."
  type        = string
  default     = "60s"
}
