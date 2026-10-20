# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Blade, Ethernal's fork of Polygon Edge (the Go module path is still `github.com/0xPolygon/polygon-edge`):
an EVM chain with PolyBFT consensus. It is the chain that hosts the Apex/Skyline bridge smart
contracts. The bridge validator/oracle itself lives in the sibling `apex-bridge` repository. This repo
contains the chain node and the end-to-end suites that run the whole bridge (Blade, Cardano, Nexus EVM,
Solana and the `apex-bridge` binary) together.

## Commands

```bash
make download-submodules   # submodules: blade-contracts, apex-bridge-smartcontracts, cardano-smart-contracts, skyline-solana-programs, tests/*
make build                 # ./blade binary with version ldflags
make lint                  # golangci-lint (v1 config, .golangci.yml, --whole-files)
make unit-test             # go test -race -shuffle=on over everything except e2e (20m timeout)
go test ./consensus/polybft/... -run TestName   # a single unit test
```

E2E tests are skipped unless `E2E_TESTS=true`; `E2E_LOGS=true` writes logs (to `e2e-logs-*` dirs).
Each Make target builds `artifacts/blade`, sets `EDGE_BINARY`, and filters by test-name prefix:

| Target | `-run` filter |
| --- | --- |
| `test-e2e-apex-bridge` | `ApexBridge_` |
| `test-e2e-apex-bridge-with-nexus` | `ApexBridgeWithNexus_` |
| `test-e2e-skyline-bridge` | `SkylineBridge_` |
| `test-e2e-skyline-bridge-mba` / `-cc` | `SkylineBridgeMBA_` / `SkylineBridgeCC_` |
| `test-e2e-skyline-solana` | `SkylineSolana_` |
| `test-e2e-refund-apex-bridge` / `-skyline-bridge` | `ApexRefund` / `SkylineRefund` |
| `test-e2e-polybft` | every e2e test except ApexBridge |

A single e2e test:

```bash
go build -o artifacts/blade . && \
EDGE_BINARY=$PWD/artifacts/blade E2E_TESTS=true E2E_LOGS=true \
  go test -v -timeout=2h ./e2e-polybft/e2e/... -run '^TestE2E_SkylineSolana_Name$'
```

Skyline targets need `npm i` in `cardano-smart-contracts/` first; the Make targets do that.

External binaries the e2e framework resolves through env vars, falling back to `PATH`
(`e2e-polybft/cardanofw/test-apex-utils.go`): `APEX_BRIDGE_BINARY` (`apex-bridge`), `BLADE_BINARY`,
`CARDANO_NODE_BINARY` / `CARDANO_NODE_11_BINARY`, `CARDANO_CLI_BINARY` / `CARDANO_CLI_11_BINARY`,
`OGMIOS` / `OGMIOS_11_BINARY`, `SPL_TOKEN_BINARY`, `SOLANA_TEST_VALIDATOR_BINARY`. The `apex-bridge` on `PATH` is often stale: check it with
`go version -m $(which apex-bridge)` or set `APEX_BRIDGE_BINARY` to a fresh build of the sibling repo.

Testnet suites (`test-testnet-*`, `fund-/defund-testnet-*`) talk to real networks and need
`E2E_TESTNET_WALLET_KEYS_PATH` (or `_CONTENT`), `TESTNET_*` API keys; don't run them unasked.

Code generation:

- `make update-apex-contracts`: regenerate Apex contract artifacts and Go bindings from
  `apex-bridge-smartcontracts` (`consensus/polybft/contractsapi/apex-artifacts-gen`, then `bindings-gen`).
- `make compile-blade-contracts` / `generate-smart-contract-bindings`: the same for `blade-contracts`.
- `make protoc`: gRPC/proto for `server`, `network`, `txpool`, `consensus/polybft`.

## Layout

- `main.go` → `command/root`: Cobra CLI. `command/server` runs the node; `command/genesis` builds the
  genesis, including the Apex parameters (`command/genesis/polybft_params.go`, `params.go`).
- `server/`: wires the node: `blockchain`, `state` (EVM executor, `state/runtime` incl. precompiles),
  `txpool`, `network` (libp2p), `jsonrpc`, `syncer`, and the consensus engine.
- `consensus/polybft/`: PolyBFT on top of go-ibft. `polybft.go` and `consensus_runtime.go` drive epochs;
  `fsm.go` builds and validates blocks; stake, governance, checkpoint and state-sync managers keep their
  state in bolt stores (`state_store_*.go`). Apex genesis contracts are deployed by
  `apex_contracts_initializer.go`; their bindings are in `contractsapi/apex_*.go`.
- `state/runtime/precompiled/apex_bls_signs_verification.go`: the BLS precompile the bridge contracts
  use to verify validator signatures.
- `e2e-polybft/`:
  - `framework/`: spins up Blade clusters (`test-cluster.go` reads `E2E_TESTS`, `E2E_LOGS`).
  - `cardanofw/`: orchestrates the whole bridge system: Cardano clusters and Ogmios, the EVM (Nexus)
    chain, the Solana chain, and `apex-bridge` validator processes (`test-apex-system.go`,
    `test-apex-bridge.go`, `test-apex-validator.go`).
  - `solanafw/`: `solana-test-validator` lifecycle and SPL helpers.
  - `e2e/`: the test suites (`apex_bridge_test.go`, `skyline_*_test.go`, `refund_*_test.go`, `testnet_*`).
- `e2e/`: legacy (pre-PolyBFT) e2e tests (`make test-e2e-legacy`); `loadtest/`: load-test runner.
- Core node packages: `accounts/`, `archive/` (chain backup/restore), `blockchain/`, `chain/` (chain
  params and fork config), `contracts/` (system addresses incl. `system_apex.go`), `crypto/`,
  `forkmanager/`, `gasprice/`, `helper/`, `jsonrpc/`, `network/`, `secrets/` (validator key managers),
  `state/`, `syncer/`, `txpool/`, `txrelayer/`, `types/`, `validate/` (proto validation), `versioning/`.
- Submodules: `apex-bridge-smartcontracts/`, `blade-contracts/`, `cardano-smart-contracts/`,
  `skyline-solana-programs/`, `tests/` (ethereum/tests and EVM benchmarks).
- `gorules/`: custom go-ruleguard lint rules; `scripts/`: cluster and fuzz scripts; `docker/`: local
  compose cluster; `licenses/`: dependency licence lists.
- Docs: `docs/` (CLI, bridge), `gitbook/` (architecture, detailed design, security), `plantuml/`
  (consensus, state and syncer diagrams).
- `.github/`: CI workflows (lint, unit, e2e suites, nightly and testnet runs), Dependabot.
- `test-logs/`: untracked local e2e logs.
- `.claude/`: Claude Code settings, core plugin's `ethernal.json`, and path-scoped `rules/`.

## Conventions

- The lint config enables `wsl` and `nlreturn`: blank lines around blocks and before `return`/`break`
  are enforced. Test helpers need `t.Helper()` (`thelper`); `godox` flags TODO/FIXME.
- Format with `gofmt -s`.
- Generated files (`contractsapi/*_sc_data.go`, `gen_sc_data.go`, bindings, `*.pb.go`) are regenerated,
  not hand-edited.
- PRs target `develop`; `feat/skyline` is the long-lived Skyline branch.

## Stack

Languages: Go.

- Go 1.24.1 (toolchain go1.24.2)

## Verification

- Before a push: `make lint` (the fast gate).
- Before a PR: `make lint build` (the full gate).
- Build (go): `go vet ./...` (or `golangci-lint run`), `go test ./...` and `govulncheck ./...`,
  each with `GOTOOLCHAIN=local`.
- Then ask Claude to run its verify skill: a Scenario, Layer, Outcome table from real output.
- Results come from real output; anything not run is reported as not run.

## Rules

- Conventional commits, `<type>(<scope>): <summary>`. Why: the squash commit takes the PR
  title, and a readable history is the project's record of changes.
- Claude verifies a task, then leaves its changes unstaged and names the files; the engineer
  commits them. Why: the engineer owns every commit.
- New environment variables go into `.env.example` with a name and no value. Why: secrets stay
  out of git and out of the model's context.
- Adopt the newest stable release of a dependency; fix the code instead of pinning back. Why:
  old versions pile up security fixes that are hard to catch up on.

## Don't

- Don't commit, push or open a PR unless asked; never push to `feat/skyline` or force-push.
- Don't merge with a red or missing required check.
- Don't read, print or commit `.env` files.
- Don't describe unverified behaviour as verified.
