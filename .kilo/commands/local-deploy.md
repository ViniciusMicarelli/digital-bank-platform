# Deploy to local Kubernetes cluster

Deploys services to a local Kind cluster with the specified cell.

## Usage

```bash
make local-deploy CELL=cell-1
```

## Prerequisites

- kubectl configured to access the Kind cluster
- Kind cluster running with at least 4 CPU cores and 8GB RAM
