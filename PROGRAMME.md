# AxionLab — Testnet Programme

> Experimental software in public validation phase.  
> Licence: MIT · Language: Go · Cryptography: Dilithium-5 (NIST FIPS 204)

---

## What is AxionLab

AxionLab is experimental software infrastructure focused on digital asset management and productive use of crypto capital.

Unlike traditional exchanges, AxionLab does not operate as a fiat on-ramp or off-ramp platform. It is a technical environment for cryptoasset holders seeking structured, utility-driven use of their resources — with post-quantum security as its foundation.

**The project is in a public validation phase. This is not a game. This is the real deployment of a post-quantum system.**

---

## Ecosystem

| Component | Function |
|-----------|----------|
| **Axion Node** | Native blockchain with QBFT consensus and Dilithium-5 signatures |
| **Pool Node** | Liquidity and multi-chain staking execution |
| **Stabilizer** | Validator node with automatic market stabilisation logic |
| **Econometer** | Sovereign oracle for pricing and network indices |
| **Vault Keeper** | Multi-chain custody gateway with rotating wallets |

### Supported external networks

Ethereum · BNB Chain · Polygon · Solana · Monad

---

## Phase 1 — Performance Validation *(current)*

**Objective:** Collect performance metrics from the network, servers and ecosystem components.

### Available

- Creation of Axion wallets with Dilithium-5 keys
- Internal swap between assets
- AXN transfer between internal wallets
- Staking and AXN yield generation
- Withdrawals to external wallets via Vault Keeper
- Block and transaction explorer

### Temporarily unavailable

- External deposits of Solana (SOL) — high latency in sending hash release
- External deposits of Monad (MONAD) — same reason

### Target and exit condition

**Target:** 1,000 active holders (wallets with AXN balance > 0)

Upon reaching the target, the following restrictions are automatically enforced:

- New wallet creation: **blocked**
- External deposits: **blocked**
- Withdrawals to external wallets: **maintained** (Vault Keeper operating normally)
- Staking on validators: **maintained**
- Internal operations (swap, AXN transfer): **maintained**
- Internal price fluctuation protection: **activated**
- Production of technical and financial reports based on collected data

---

## Transition Between Phases

The transition to Phase 2 is **not automatic**. It will depend on the full analysis of data collected during Phase 1, including:

- Validator server performance
- QBFT consensus stability
- Econometer metrics (indices, score, gas fees)
- Identified fixes across components

---

## Phase 2 — Security and Capacity Validation *(planned)*

**Objective:** Expand network capacity with dedicated infrastructure and apply in-depth security fixes.

### Planned changes

- Deployment of new validator servers with higher computational capacity
- Migration of the core node to dedicated infrastructure
- Vault Keeper upgrade (faster deposit and withdrawal detection, enhanced security)
- Fixes to Econometer, Stabilizer and Pool Node based on Phase 1 learnings
- Beginning of the process to seek regulatory approval in a jurisdiction to be determined

---

## Transparency and Reports

All technical and financial reports will be public. Performance results, indices, Econometer metrics and deployment progress will be shared continuously in this repository and on the official website.

The ecosystem source code is available in this repository under the MIT licence.

---

## How to Participate

1. Visit [www.testnet-axion.org](https://www.testnet-axion.org)
2. Read the technical notices in the **Notices** section before creating your wallet
3. Create your wallet — you will receive a JSON file and an authentication code; **store them safely, there is no recovery without them**
4. Explore internal swaps, AXN transfers between wallets, and staking
5. Track your yield and network status on the block explorer
6. Contribute bug reports, tests and suggestions via GitHub Issues

### For developers

Developers, researchers and market practitioners who identify relevant improvements are welcome to collaborate. Submit your contribution proposal including:

1. Who you are (individual or team)
2. What you intend to build or improve
3. Technical scope and expected impact
4. Relevant references or portfolio

---

## Official Channels

| Channel | Link |
|---------|------|
| Website | [www.testnet-axion.org](https://www.testnet-axion.org) |
| GitHub | [github.com/AxionLab-Testnet](https://github.com/AxionLab-Testnet) |
| Instagram | [@testnet_axion](https://www.instagram.com/testnet_axion/) |

---

*Built with Go + Dilithium-5 · Open source · Post-quantum security · Community first*

*Last updated: May 2026*