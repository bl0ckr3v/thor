# VeChain Thor PoC Setup Instructions

## Quick Start Guide

Follow these steps to set up and run the VeChain Thor MTT Fee Delegation Abuse PoC.

### Prerequisites

1. **VeChain Thor Node**
   - Go 1.19+ and C compiler installed
   - VeChain Thor repository cloned and built

2. **Node.js Environment**
   - Node.js v18+ with npm
   - TypeScript support

### Step 1: Build VeChain Thor

From the main thor repository directory:

```bash
# Build the Thor node
make all

# Verify build
./bin/thor --help
```

### Step 2: Start Thor Solo Node

Start an isolated Thor Solo node for testing:

```bash
# Start solo node (runs on localhost:8669)
./bin/thor solo --api-addr 0.0.0.0:8669

# In another terminal, verify node is running
curl http://localhost:8669/blocks/best
```

Expected output: JSON response with block information

### Step 3: Install PoC Dependencies

Navigate to the PoC directory and install dependencies:

```bash
cd poc/
npm install
```

### Step 4: Run the PoC

Execute the main exploit demonstration:

```bash
# Run the basic PoC
npm run poc

# Or run the comprehensive test suite
npm run test
```

## Detailed Setup

### Thor Solo Node Configuration

The Thor Solo node provides an isolated blockchain environment perfect for PoC testing:

- **Network**: Private solo chain
- **API Endpoint**: http://localhost:8669
- **Consensus**: Single node (no mining delay)
- **Accounts**: Pre-funded development accounts
- **Builtin Contracts**: All VeChain builtin contracts deployed

### PoC Components

The PoC includes several components:

1. **Main Exploit Script** (`mtt-delegation-abuse.ts`)
   - Demonstrates the core vulnerability
   - Creates MTT with fee delegation abuse
   - Validates unauthorized access

2. **Test Suite** (`test-runner.ts`)
   - Comprehensive vulnerability testing
   - Multiple attack scenarios
   - Mitigation effectiveness analysis

3. **Vulnerable Contracts** (`contracts/`)
   - Example vulnerable application
   - Malicious exploit contract
   - Demonstrates real-world impact

4. **Utilities** (`utils/`)
   - Thor API client
   - Contract deployment helpers
   - Account management

### Expected Behavior

When running the PoC, you should see:

1. **Setup Phase**
   - Connection to Thor Solo node
   - Account initialization
   - Contract deployment

2. **Exploit Phase**
   - MTT construction with fee delegation
   - Transaction submission and confirmation
   - Context abuse demonstration

3. **Verification Phase**
   - Transaction analysis
   - Vulnerability confirmation
   - Impact assessment

### Troubleshooting

#### Thor Node Issues

**Problem**: Cannot connect to Thor node
```
❌ Cannot connect to Thor node. Please ensure Thor Solo is running on localhost:8669
```

**Solution**:
1. Verify Thor Solo is running: `curl http://localhost:8669/blocks/best`
2. Check if port 8669 is available: `netstat -an | grep 8669`
3. Restart Thor Solo with correct parameters

#### Dependency Issues

**Problem**: TypeScript compilation errors
```
error TS2307: Cannot find module '@vechain/sdk-core'
```

**Solution**:
1. Ensure all dependencies are installed: `npm install`
2. Check Node.js version: `node --version` (should be v18+)
3. Clear npm cache: `npm cache clean --force`

#### Transaction Issues

**Problem**: Transaction fails or reverts
```
❌ Transaction reverted - exploit failed
```

**Solution**:
1. Check account balances (need VET for gas)
2. Verify contract addresses are correct
3. Increase gas limit in transaction
4. Check Thor Solo node logs for errors

### Advanced Configuration

#### Custom Network Parameters

To test with custom network parameters, modify the genesis configuration:

```bash
# Edit genesis file
vim cmd/thor/genesis/dev.go

# Rebuild Thor
make all

# Start with custom genesis
./bin/thor solo --network dev --api-addr 0.0.0.0:8669
```

#### Contract Compilation

To compile custom vulnerable contracts:

```bash
# Install Solidity compiler
npm install -g solc

# Compile contracts
solc --bin --abi contracts/VulnerableApp.sol -o contracts/compiled/

# Update contract addresses in PoC scripts
```

#### Extended Testing

For extended vulnerability testing:

```bash
# Run with verbose logging
DEBUG=* npm run poc

# Generate detailed reports
npm run test > results/test-report.txt

# Analyze transaction traces
curl http://localhost:8669/transactions/{txId} | jq '.'
```

## Security Considerations

### Isolated Environment

This PoC is designed to run in an isolated environment:

- **Thor Solo Node**: Private blockchain, no external connectivity
- **Test Accounts**: Generated keys, no real value
- **Simulated Contracts**: Demonstration purposes only

### Responsible Use

- **Do not run against mainnet or testnet** without proper authorization
- **Use only for security research** and vulnerability disclosure
- **Follow responsible disclosure** practices when reporting findings
- **Respect VeChain's bug bounty program** guidelines

### Data Privacy

- All test data remains local to your machine
- No sensitive information is transmitted
- Private keys are generated for testing only
- Transaction data is isolated to the solo node

## Support

If you encounter issues with the PoC setup:

1. **Check Prerequisites**: Ensure all required software is installed
2. **Review Logs**: Check Thor Solo node logs for errors
3. **Verify Network**: Confirm Thor API is accessible
4. **Test Components**: Run individual components to isolate issues

For questions about the vulnerability or responsible disclosure:

- Follow VeChain's security reporting guidelines
- Use official channels for vulnerability reports
- Include PoC results and detailed analysis

## Next Steps

After successfully running the PoC:

1. **Analyze Results**: Review transaction traces and vulnerability confirmation
2. **Document Findings**: Prepare detailed vulnerability report
3. **Test Mitigations**: Experiment with proposed fixes
4. **Responsible Disclosure**: Report findings through appropriate channels

The PoC demonstrates a real vulnerability in VeChain Thor's architecture that requires immediate attention and mitigation.