# VeChain Thor MTT Fee Delegation Abuse PoC

## Overview

This Proof of Concept demonstrates a critical vulnerability in VeChain Thor's Multi-Task Transaction (MTT) architecture that enables fee delegation context abuse. The vulnerability allows malicious contracts to gain unauthorized access to sponsored services by exploiting the shared transaction context across MTT clauses.

## Vulnerability Details

**CVE**: Pending
**Severity**: High
**Impact**: Direct theft of user funds through unauthorized access to sponsored services
**Root Cause**: `ExtensionV2.txGasPayer()` returns transaction-level gas payer without caller validation

### Technical Details

1. **Vulnerable Function**: `builtin/gen/extension-v2.sol:11-13`
   ```solidity
   function txGasPayer() public view returns(address) {
       return ExtensionV2Native(this).native_txGasPayer();
   }
   ```

2. **Native Implementation**: `builtin/extension_native.go:132-135`
   ```go
   {"native_txGasPayer", func(env *xenv.Environment) []any {
       output := env.TransactionContext().GasPayer  // No validation!
       return []any{output}
   }},
   ```

3. **Shared Context**: All clauses in MTT share the same `TransactionContext` including gas payer

## Prerequisites

1. **VeChain Thor Node**: Running on localhost:8669 (Solo mode recommended)
2. **Node.js**: v18+ with npm
3. **TypeScript**: For running the PoC scripts

## Setup Instructions

### 1. Start VeChain Thor Solo Node

From the main thor directory:
```bash
# Build thor if not already built
make all

# Start solo node for isolated testing
./bin/thor solo --api-addr 0.0.0.0:8669
```

The node will be accessible at `http://localhost:8669`

### 2. Install PoC Dependencies

```bash
cd poc/
npm install
```

### 3. Deploy Vulnerable Contract (Optional)

If testing against a custom vulnerable contract:
```bash
npm run deploy-vulnerable
```

## Running the PoC

### Basic Exploit Demonstration
```bash
npm run poc
```

### Full Test Suite
```bash
npm run test
```

## Expected Output

The PoC will demonstrate:

1. **Setup Phase**: Deploy contracts and fund accounts
2. **Exploit Phase**: Execute MTT with fee delegation abuse
3. **Verification Phase**: Confirm unauthorized access and fund theft

Expected console output:
```
🚀 VeChain Thor MTT Fee Delegation Abuse PoC
============================================

📋 Setup Phase:
✅ Connected to Thor Solo node at http://localhost:8669
✅ Attacker account: 0x1234...
✅ Sponsor account: 0x5678...
✅ Vulnerable app deployed at: 0x9abc...

⚡ Exploit Phase:
🔍 Building MTT with 2 clauses...
📤 Submitting transaction with fee delegation...
⏳ Waiting for transaction confirmation...
✅ Transaction confirmed: 0xdef0...

🔍 Verification Phase:
❌ VULNERABILITY CONFIRMED:
   - Clause 0: Malicious contract called txGasPayer() → returned sponsor
   - Clause 1: Gained unauthorized access to premium service
   - Result: 1000 VTHO stolen from sponsored service

💰 Impact Summary:
   - Funds stolen: 1000 VTHO
   - Unauthorized access: ✅ Confirmed
   - Sponsor unaware: ✅ Context abuse successful
```

## Files Structure

```
poc/
├── README.md                    # This file
├── package.json                 # Dependencies
├── mtt-delegation-abuse.ts      # Main PoC script
├── test-runner.ts              # Comprehensive test suite
├── contracts/                   # Vulnerable contract examples
│   ├── VulnerableApp.sol       # Example vulnerable application
│   └── MaliciousContract.sol   # Exploit contract
├── utils/                       # Helper utilities
│   ├── thor-client.ts          # VeChain Thor API client
│   ├── contract-deployer.ts    # Contract deployment helper
│   └── account-manager.ts      # Account management utilities
└── results/                     # Test results and logs
    ├── exploit-trace.json      # Detailed execution trace
    └── vulnerability-report.md # Generated vulnerability report
```

## Mitigation

The vulnerability can be mitigated by:

1. **Add caller validation to `txGasPayer()`**:
   ```solidity
   function txGasPayer() public view returns(address) {
       address payer = ExtensionV2Native(this).native_txGasPayer();
       // Add validation logic here
       require(hasRelationship(msg.sender, payer), "Unauthorized caller");
       return payer;
   }
   ```

2. **Implement context isolation for MTT clauses**
3. **Add native call validation framework**

## Responsible Disclosure

This PoC is created for responsible security research and should only be used on isolated test networks. Do not run against mainnet or testnet without proper authorization.

## References

- [VeChain Thor Repository](https://github.com/vechain/thor)
- [VeChain Developer Documentation](https://docs.vechain.org/)
- [Immunefi Bug Bounty Program](https://immunefi.com/bounty/vechain/)
- [VeChain SDK Documentation](https://docs.vechain.org/developer-resources/sdks-and-providers)