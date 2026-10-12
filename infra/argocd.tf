# --- Argo CD ------------------------------------------------------------------

locals {
  # Only register repo credentials when a token is given (private repos).
  argocd_repositories = var.git_token == "" ? {} : {
    app-repo = {
      type     = "git"
      url      = var.git_repo_url
      username = var.git_username
      password = var.git_token
    }
  }
}

resource "helm_release" "argocd" {
  name             = "argocd"
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  version          = var.argocd_chart_version
  namespace        = "argocd"
  create_namespace = true
  wait             = true
  timeout          = 900

  values = [yamlencode({
    dex           = { enabled = false }
    notifications = { enabled = false }

    configs = {
      # Serve the UI over plain HTTP; it is only bound to localhost.
      params = { "server.insecure" = true }
      cm     = { "timeout.reconciliation" = var.argocd_poll_interval }

      repositories = local.argocd_repositories
    }

    server = {
      service = {
        type         = "NodePort"
        nodePortHttp = local.argocd_node_port
      }
    }
  })]
}

# The Application is installed through the argocd-apps chart rather than a raw
# kubernetes_manifest: the Application CRD only exists once Argo CD is installed,
# and kubernetes_manifest needs the CRD at plan time.
resource "helm_release" "argocd_apps" {
  name       = "argocd-apps"
  repository = "https://argoproj.github.io/argo-helm"
  chart      = "argocd-apps"
  version    = var.argocd_apps_chart_version
  namespace  = "argocd"

  values = [yamlencode({
    applications = {
      status-check = {
        namespace = "argocd"
        project   = "default"
        source = {
          repoURL        = var.git_repo_url
          targetRevision = var.git_revision
          path           = var.app_path
        }
        destination = {
          server    = "https://kubernetes.default.svc"
          namespace = var.app_namespace
        }
        syncPolicy = {
          # Git is the source of truth: apply new commits, delete removed
          # resources, and revert manual changes made with kubectl.
          automated = {
            prune    = true
            selfHeal = true
          }
          syncOptions = ["CreateNamespace=true"]
          retry = {
            limit   = 5
            backoff = { duration = "10s", factor = 2, maxDuration = "3m" }
          }
        }
      }
    }
  })]

  # The registry redirect must be in place before Argo CD pulls the first image.
  depends_on = [helm_release.argocd, terraform_data.registry_hosts]
}
