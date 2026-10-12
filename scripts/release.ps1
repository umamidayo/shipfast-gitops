<#
.SYNOPSIS
  Build the status-check image, push it to the local Kind registry, and pin the
  new tag in deploy/overlays/local. Argo CD deploys it once the commit is pushed.

.EXAMPLE
  ./scripts/release.ps1            # build, push image, commit, git push
  ./scripts/release.ps1 -NoPush    # same, but leave the commit for you to push
#>
param(
    [switch]$NoPush,
    [string]$Registry = $(if ($env:REGISTRY) { $env:REGISTRY } else { 'localhost:5001' })
)
$ErrorActionPreference = 'Stop'

function Invoke-Native {
    & $args[0] $args[1..($args.Count - 1)]
    if ($LASTEXITCODE -ne 0) { throw "'$($args -join ' ')' failed with exit code $LASTEXITCODE" }
}

$repoRoot = (git rev-parse --show-toplevel)
Set-Location $repoRoot
$overlay = 'deploy/overlays/local/kustomization.yaml'

# The image tag is the commit that produced it, so uncommitted app changes
# would make the tag lie about its contents.
if (git status --porcelain -- app) {
    throw 'app/ has uncommitted changes; commit them first so the image tag matches its source.'
}

$tag = (git rev-parse --short HEAD)
$image = "$Registry/status-check:$tag"

Write-Host "==> Building $image"
Invoke-Native docker build --build-arg "VERSION=$tag" -t $image app

Write-Host "==> Pushing $image"
Invoke-Native docker push $image

Write-Host "==> Pinning $tag in $overlay"
$path = Join-Path $repoRoot $overlay
$content = [IO.File]::ReadAllText($path) -replace '(?m)^(\s*newTag:).*$', "`$1 `"$tag`""
[IO.File]::WriteAllText($path, $content)   # UTF-8 without BOM

Invoke-Native git add $overlay
git diff --cached --quiet
if ($LASTEXITCODE -eq 0) {
    Write-Host "Overlay already at $tag; nothing to commit."
    exit 0
}
Invoke-Native git commit -m "deploy(local): status-check $tag"

if ($NoPush) {
    Write-Host "Committed. Run 'git push' to deploy."
} else {
    Write-Host '==> Pushing to git; Argo CD will sync on its next poll'
    Invoke-Native git push
}
