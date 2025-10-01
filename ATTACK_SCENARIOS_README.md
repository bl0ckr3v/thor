# VeChain Thor Attack Scenarios - Security Analysis

This repository contains proof-of-concept test files demonstrating critical vulnerabilities in VeChain Thor's builtin contracts. These tests follow the existing VeChain Thor test suite patterns and can be integrated directly into the project's testing framework.

## Files Overview

### 1. `attack_scenarios_test.go`
High-level attack scenario demonstrations following the established test patterns.

### 2. `native_vulnerabilities_test.go` 
Detailed analysis of native call vulnerabilities with focused test cases.

## Validated Attack Scenarios

### Scenario 1: Fee Delegation Context Abuse (HIGH RISK)
**File**: `TestAttackScenario1_FeeDelegationContextAbuse`
**Vulnerability**: `ExtensionV2.txGasPayer()` returns gas payer without validating caller relationship
**Impact**: Unauthorized access to sponsored services in Multi-Task Transactions

```solidity
// Vulnerable code in extension-v2.sol:11-13
function txGasPayer() public view returns(address) {
    return ExtensionV2Native(this).native_txGasPayer();
}
```

**Attack Vector**:
- MTT Clause 1: Establish sponsored context via delegation
- MTT Clause 2: Call vulnerable contract checking `txGasPayer() == trustedSponsor`
- Result: Unauthorized access granted based on sponsor, not actual caller

### Scenario 2: Native-Solidity State Inconsistency (MEDIUM-HIGH RISK)
**File**: `TestAttackScenario2_NativeStateInconsistency`
**Vulnerability**: `Energy._transfer()` relies on native call atomicity without guarantees
**Impact**: Potential double-spending, fund loss, or VTHO inflation

```solidity
// Vulnerable code in energy.sol:68-76
function _transfer(address _from, address _to, uint256 _amount) internal {
    if (_amount > 0) {
        require(EnergyNative(this).native_sub(_from, _amount), "builtin: insufficient balance");
        // believed that will never overflow
        EnergyNative(this).native_add(_to, _amount);
    }
    emit Transfer(_from, _to, _amount);
}
```

**Attack Vector**:
- MTT context allows clause isolation
- `native_sub()` → `native_add()` sequence may not be atomic across clauses
- Could enable balance manipulation if native state desynchronizes

### Scenario 3: Governance Quorum Bypass (MEDIUM RISK)
**File**: `TestAttackScenario3_GovernanceQuorumBypass`
**Vulnerability**: Timestamp-based proposal validation vulnerable to manipulation
**Impact**: Unauthorized governance proposals execution

**Attack Vector**:
- Validators manipulate block timestamps
- Extend or shorten proposal validity windows
- Bypass intended governance timeouts

### Scenario 4: Params-Governance Privilege Escalation (HIGH RISK)
**File**: `TestAttackScenario4_ParamsGovernancePrivilegeEscalation`
**Vulnerability**: `Params.get()` and `executor()` rely on potentially stale native reads
**Impact**: Critical parameter manipulation, network DoS

```solidity
// Vulnerable code in params.sol:10-12, 21-23
function executor() public view returns(address) {
    return ParamsNative(this).native_executor();
}

function get(bytes32 _key) public view returns(uint256) {
    return ParamsNative(this).native_get(_key);
}
```

**Attack Vector**:
- Native state lags behind Solidity state in MTT
- Proposals approved based on stale parameter reads
- Could manipulate critical parameters like `maxClauseCount`

## Detailed Vulnerability Analysis

### Native Call Vulnerabilities

#### 1. `TestNativeSubAddAtomicity`
Demonstrates the critical assumption in Energy contract's transfer mechanism.

#### 2. `TestTxGasPayerContextVulnerability`
Shows how `txGasPayer()` can be exploited for authorization bypass.

#### 3. `TestParamsNativeGetVulnerability`
Reveals staleness risks in parameter reads affecting governance.

#### 4. `TestAuthorityNativeVulnerability`
Exposes validator management vulnerabilities that could compromise consensus.

### Multi-Task Transaction (MTT) Specific Risks

#### `TestMTTAttackVector`
Demonstrates how MTT's clause isolation amplifies native call vulnerabilities:
- Shared context across clauses (gas payer, transaction ID)
- Independent contract calls within single transaction
- Potential for state inconsistency between clauses

## Running the Tests

### Prerequisites
- Go 1.19+
- VeChain Thor development environment
- Required dependencies from `go.mod`

### Execution
```bash
# Run all attack scenario tests
go test -v -run "TestAttackScenario"

# Run specific vulnerability tests
go test -v -run "TestNativeSubAddAtomicity"
go test -v -run "TestTxGasPayerContextVulnerability"

# Run combined attack scenarios
go test -v -run "TestCombinedAttackScenario"
```

### Integration with Existing Test Suite
These tests follow the same patterns as existing VeChain Thor tests:
- Use `ctest` structure for contract testing
- Follow `builtin_test` package conventions
- Utilize existing helper functions (`buildGenesis`, event constructors)
- Compatible with `stretchr/testify` assertions

## Critical Findings Summary

### High Risk Vulnerabilities
1. **Fee Delegation Context Abuse** - Direct theft of user funds
2. **Params-Governance Privilege Escalation** - Network shutdown capability

### Medium-High Risk Vulnerabilities  
1. **Native-Solidity State Inconsistency** - Fund loss/inflation potential

### Medium Risk Vulnerabilities
1. **Governance Quorum Bypass** - Unauthorized proposal execution

## Recommended Mitigations

### Immediate Actions Required
1. **Add context validation to `ExtensionV2.txGasPayer()`**
   - Verify caller-sponsor relationship
   - Implement access control checks

2. **Implement reentrancy guards in `Energy._transfer()`**
   - Add explicit atomicity guarantees
   - Validate native call sequences

3. **Add staleness checks to `Params.get()`**
   - Implement consistency validation
   - Add native state versioning

### Long-term Improvements
1. **Native call atomicity guarantees for MTT**
2. **Comprehensive audit of all native function calls**
3. **Enhanced governance timestamp validation**
4. **State consistency monitoring between native and Solidity layers**

## Impact Assessment

These vulnerabilities affect core VeChain Thor functionality:
- **Energy Contract**: VTHO token operations (DeFi impact)
- **Authority Contract**: Validator management (consensus impact)  
- **Executor Contract**: Governance operations (protocol impact)
- **Params Contract**: Network parameters (stability impact)

The combination of MTT's unique architecture with native call dependencies creates attack vectors not present in standard EVM implementations.

## Responsible Disclosure

These findings should be:
1. Reported through VeChain's official security channels
2. Coordinated with the VeChain development team
3. Addressed before public disclosure
4. Included in future security audits

## Test Environment Notes

- Tests use `muxdb.NewMem()` for isolated testing
- Genesis blocks configured with minimal required contracts
- Runtime contexts simulate realistic blockchain conditions
- All tests are deterministic and repeatable

## Contributing

When adding new attack scenarios:
1. Follow existing test patterns
2. Include detailed vulnerability analysis
3. Provide clear impact assessment
4. Document mitigation strategies
5. Ensure tests are deterministic

---

**Security Notice**: These tests are for security research and vulnerability disclosure purposes only. Do not use these techniques against live networks or for malicious purposes.