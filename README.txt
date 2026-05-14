Axion Lab — Liquid Staking with Post-Quantum Security

Official Website: www.testnet-axion.org
Contact: axionlab@hotmail.com

Axion Lab is a liquid staking platform that connects Solana, Ethereum, BNB Chain, Polygon and Monad to its own ecosystem with native post-quantum security (Dilithium-5 / FIPS 204).

Status: Public validation phase — open source, community-driven testing


COMPONENTS

axion-node: Native blockchain with QBFT consensus and Dilithium-5 signatures
pool-node: Liquidity and multi-chain staking execution
stabilizer: Validator node with automatic market stabilization logic
econometer: Sovereign oracle for pricing and network indices
vault-keeper: Multi-chain custody gateway with rotating wallets


TECHNOLOGIES

- Go — all components
- Dilithium-5 — post-quantum signatures (NIST FIPS 204)
- Kyber-1024 — key encapsulation
- QBFT — consensus with validator rotation
- BoltDB — local persistence


SUPPORTED NETWORKS

- Solana
- Ethereum
- BNB Chain
- Polygon
- Monad


HOW TO RUN

Each component has its own README with detailed instructions. In short:

git clone https://github.com/AxionLab/axion-node.git
cd axion-node
go build -o axion-node ./cmd/axion-node/
./axion-node --auto --validator --rpc-port 8089 (first node)
./axion-node --auto --validator --bootstrap FIRST_NODE_IP:8089 (secondary node)


ARCHITECTURE

Econometer (oracle) provides price, indices and gas fees to Axion Node, Pool and Stabilizer.
Vault Keeper acts as the multi-chain custody gateway for all components.


SECURITY

- Post-quantum signatures (Dilithium-5)
- Private keys encrypted with AES-256-GCM + Argon2id
- 2FA (TOTP) for sensitive operations
- IP whitelisting
- Rate limiting


LICENSE

MIT


CONTACT

X: @AxionLab
GitHub: AxionLab
Email: axionlab@hotmail.com
Website: www.testnet-axion.org


DISCLAIMER

This project is in public validation phase. It is experimental software. Use at your own risk. We do not recommend using it with real funds of significant value until independent security audits are completed.

Built with Go + Dilithium-5. Open source, post-quantum security, community first.