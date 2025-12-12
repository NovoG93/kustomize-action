[![codecov](https://codecov.io/gh/NovoG93/kusotmize-action/graph/badge.svg)](https://codecov.io/gh/NovoG93/kusotmize-action)

# kustomize-action

A GitHub Action designed to automatically detect and build "root" Kustomize configurations within a repository.

This action scans your directory structure to identify top-level `kustomization.yaml` (or `.yml`) files—ignoring nested bases or overlays that are arguably included by a parent—and renders the manifests for usage in subsequent workflow steps.

## How it Works

The action determines which directories are "roots" based on the presence of Kustomize files in the directory hierarchy:

1.  **Scanning:** It scans the provided path (defaults to the repo root) for `kustomization.yaml/yml` files.
2.  **Root Logic:** A directory is considered a **root** if it contains a kustomization file, but **none of its ancestor directories** contain one.
3.  **Assumption:** This logic assumes that if a parent directory has a kustomization file, it is responsible for including/building the nested sub-directories.

### Directory Structure Example

<details open>

<summary><b>Click to see logic visualization</b></summary>

In the example below, `✅` denotes a detected root, while `❌` denotes a nested file that will be skipped (unless `build-all` is enabled).

```shell
.
├── apps
│   ├── couchdb                 # ✅ ROOT: No kustomization in parent 'apps'
│   │   └── kustomization.yaml
│   └── immich                  # ✅ ROOT: No kustomization in parent 'apps'
│       ├── kustomization.yaml
│       └── cloudnative-pg      # ❌ SKIP: Parent 'immich' has kustomization
│           └── kustomization.yaml
└── core
    ├── calico                  # ✅ ROOT
    │   └── kustomization.yaml
    └── metallb-system          # ✅ ROOT
        └── kustomization.yaml
```

</details>

-----

## Usage

### Minimal Configuration

Add the following step to your workflow to build all detected roots using default settings.

```yaml
name: Build kustomize roots
on: [push]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build all root kustomizations
        uses: novog93/kustomize-action@main
```

### Advanced Configuration

Override defaults to specify versions, output directories, or Helm integration.

```yaml
- name: Build with Custom Settings
  uses: novog93/kustomize-action@main
  with:
    kustomize-version: '5.4.3'
    enable-helm: 'true'
    build-all: 'false'
    output-dir: './manifests'
```

-----

## ⚙️ Inputs

| Input | Description | Default |
| :--- | :--- | :--- |
| `output-dir` | Directory where rendered manifests will be written (when output=files). | `./kustomize-builds` |
| `kustomize-version` | The specific version of Kustomize to use (e.g., `5.4.3`). | *Latest* |
| `kustomize-sha256` | Optional SHA256 of the downloaded kustomize tarball (hex, supports `sha256:` prefix). | *(empty)* |
| `enable-helm` | Enable Helm chart inflation generator support. | `true` |
| `load-restrictor` | Setting for `kustomize build --load-restrictor`. | `LoadRestrictionsNone` |
| `build-all` | If `true`, builds **every** found kustomization file, ignoring the "root" logic. | `false` |
| `fail-fast` | If `true`, cancel remaining builds on first failure. | `false` |
| `fail-on-error` | If `true`, exit non-zero when any build fails. | `false` |

## 📦 Outputs

This action produces the following outputs which can be used in subsequent steps:

| Output | Description |
| :--- | :--- |
| `artifact-name` | Name of the artifact folder containing the rendered manifests. |
| `manifest-count` | The total number of manifests generated. |
| `success-count` | The number of kustomizations successfully built. |
| `fail-count` | The number of builds that failed. |
| `roots-json` | A JSON array containing the paths of all discovered root kustomization files relative to the repo root. |

-----

## 🛠️ Development

<details>
<summary><b>Expand for Local Testing Instructions</b></summary>

### Prerequisites

  * Go 1.25+
  * Git

### 1. Setup

Clone the repository and download dependencies:

```bash
git clone <repo-url>
cd src
go mod download
```

### 2. Build & Run

**macOS**

```bash
# Build (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ../action .

# Build (Intel)
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o ../action .

# Copy to test repo
cp ../action <path-to-repo-with-manifests>
cd <path-to-repo-with-manifests>

# Run Default
./action

# Run Custom
KUSTOMIZE_VERSION="v5.6.1" BUILD_ALL="true" FAIL_ON_ERROR="true" FAIL_FAST="true" ./action
```

**Windows (PowerShell)**

```pwsh
# Build
$env:CGO_ENABLED=0; $env:GOOS="windows"; $env:GOARCH="amd64"; go build -o ../action.exe .

# Copy to test repo
cp ../action.exe <path-to-repo-with-manifests>
cd <path-to-repo-with-manifests>

# Run Default
.\action.exe

# Run Custom
$env:KUSTOMIZE_VERSION="v5.6.1"; $env:BUILD_ALL="true"; $env:FAIL_ON_ERROR="true"; $env:FAIL_FAST="true"; .\action.exe
```

**Linux**

```bash
# Build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../action .

# Copy to test repo
cp ../action <path-to-repo-with-manifests>
cd <path-to-repo-with-manifests>

# Run Default
./action

# Run Custom
KUSTOMIZE_VERSION="v5.6.1" BUILD_ALL="true" FAIL_ON_ERROR="true" FAIL_FAST="true" ./action
```

### 3. Testing

Run the test suite:

```bash
make test
```

Run tests with coverage reporting:

```bash
make coverage
```

Clean up build artifacts and coverage files:

```bash
make clean
```

</details>

