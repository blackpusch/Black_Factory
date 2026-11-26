# GengiSWAP

## Overview
Deterministic task execution engine and protocol suite for Black Factory. GengiSWAP coordinates Chef, Preparador, and Auditor roles through structured task artifacts, immutable logs, and Go-based ring state management.

## Subdirectories & Boundaries
- engine: [./engine/README.md](./engine/README.md) - Deterministic Go engine and CLI (`gengis.exe`) implementing scope enforcement, QA gates, and workspace lifecycle.
- templates: [./templates/README.md](./templates/README.md) - Canonical task templates (`auditoria_e2e`, `mapeo_estatico`, `correccion_hallazgos`, `modulo_nuevo`).
