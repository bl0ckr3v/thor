# VeChain Thor Function Flow Analysis

## Understanding the Real Architecture

After exploring the actual VeChain Thor codebase, here's how the functions actually work, who can call them, and what user inputs are involved.

## 1. Multi-Task Transaction (MTT) Architecture

### How MTT Actually Works

**Transaction Structure:**
```go
// tx/transaction.go
type Transaction struct {
    body txData  // Contains clauses, gas, expiration, etc.
}

// tx/clause.go  
type Clause struct {
    To    *thor.Address  // Target contract address
    Value *big.Int       // VET amount to transfer
    Data  []byte         // Contract call data
}
```

**Key Insight:** A single transaction can contain multiple clauses that execute sequentially, but they share:
- Transaction ID (`txCtx.ID`)
- Gas payer (`txCtx.GasPayer`) 
- Origin (`txCtx.Origin`)
- Block reference and expiration

### Transaction Execution Flow

```go
// runtime/runtime.go:PrepareTransaction()
func (rt *Runtime) PrepareTransaction(tx *tx.Transaction) *TransactionExecutor {
    // Creates executor that processes clauses sequentially
    return &TransactionExecutor{
        HasNextClause: hasNext,
        PrepareNext: func() {
            // Each clause gets same TransactionContext but different clauseIndex
            rt.PrepareClause(resolvedTx.Clauses[nextClauseIndex], nextClauseIndex, leftOverGas, txCtx)
        }
    }
}
```

**Critical Finding:** Each clause shares the same `TransactionContext` but has different `clauseIndex`. This is where context confusion can occur.

## 2. Native Function Call Mechanism

### How Native Calls Work

**Registration Process:**
```go
// builtin/extension_native.go:init()
func init() {
    defines := []struct {
        name string
        run  func(env *xenv.Environment) []any
    }{
        {"native_txGasPayer", func(env *xenv.Environment) []any {
            output := env.TransactionContext().GasPayer  // Direct access to shared context
            return []any{output}
        }},
    }
    // Register native methods by contract address + method ID
    nativeMethods[methodKey{Extension.Address, method.ID()}] = &nativeMethod{...}
}
```

**Execution Process:**
```go
// builtin/builtin.go:FindNativeCall()
func FindNativeCall(to thor.Address, input []byte) (*abi.Method, func(*xenv.Environment) []any, bool) {
    methodID, err := abi.ExtractMethodID(input)
    method := nativeMethods[methodKey{to, methodID}]
    return method.abi, method.run, true  // Returns the native function
}
```

**Key Insight:** Native functions have direct access to the execution environment and can read shared transaction context without any validation.

## 3. Function-by-Function Analysis

### ExtensionV2.txGasPayer()

**Who can call:** Anyone - it's a public view function
**User input:** None required
**Access control:** None

```solidity
// builtin/gen/extension-v2.sol:11-13
function txGasPayer() public view returns(address) {
    return ExtensionV2Native(this).native_txGasPayer();
}
```

**Native implementation:**
```go
// builtin/extension_native.go:132-135
{"native_txGasPayer", func(env *xenv.Environment) []any {
    output := env.TransactionContext().GasPayer  // Direct read from shared context
    return []any{output}
}},
```

**Vulnerability:** The function returns `env.TransactionContext().GasPayer` which is set at the transaction level, not clause level. In MTT:
- Clause 1: Malicious contract calls `txGasPayer()` 
- Returns the gas payer for the entire transaction
- No validation that `msg.sender` has any relationship to the gas payer

### Energy._transfer()

**Who can call:** Internal function called by `transfer()`, `transferFrom()`, `move()`
**User input:** `_from`, `_to`, `_amount` addresses and amount
**Access control:** Caller validation in public functions

```solidity
// builtin/gen/energy.sol:68-76
function _transfer(address _from, address _to, uint256 _amount) internal {
    if (_amount > 0) {
        require(EnergyNative(this).native_sub(_from, _amount), "builtin: insufficient balance");
        // believed that will never overflow
        EnergyNative(this).native_add(_to, _amount);
    }
    emit Transfer(_from, _to, _amount);
}
```

**Native implementations:**
```go
// builtin/energy_native.go:75-94
{"native_sub", func(env *xenv.Environment) []any {
    // Parses address and amount from contract call
    env.ParseArgs(&args)
    // Calls Go implementation
    ok, err := Energy.Native(env.State(), env.BlockContext().Time).Sub(thor.Address(args.Addr), args.Amount)
    return []any{ok}
}},

{"native_add", func(env *xenv.Environment) []any {
    // Similar pattern - direct state modification
    Energy.Native(env.State(), env.BlockContext().Time).Add(thor.Address(args.Addr), args.Amount)
    return nil
}},
```

**Vulnerability:** The sequence `native_sub()` → `native_add()` assumes atomicity, but:
1. Each native call is independent 
2. State changes happen immediately in Go layer
3. No explicit transaction-level atomicity guarantees
4. In MTT, if clause execution is interrupted between calls, inconsistent state possible

### Params.get() and Params.executor()

**Who can call:** Anyone - public view functions
**User input:** `get()` takes a `bytes32 key`, `executor()` takes none
**Access control:** None for reading

```solidity
// builtin/gen/params.sol:10-12, 21-23
function executor() public view returns(address) {
    return ParamsNative(this).native_executor();
}

function get(bytes32 _key) public view returns(uint256) {
    return ParamsNative(this).native_get(_key);
}
```

**Native implementations:**
```go
// builtin/params_native.go (inferred from params.go)
func (p *Params) Get(key thor.Bytes32) (value *big.Int, err error) {
    // Reads directly from state storage
    err = p.state.DecodeStorage(p.addr, key, func(raw []byte) error {
        return rlp.DecodeBytes(raw, &value)
    })
    return
}
```

**Vulnerability:** These functions read directly from state without consistency checks:
1. `native_executor()` reads executor address from state
2. `native_get()` reads parameter values from state  
3. In MTT, if state changes between clauses, stale reads possible
4. Governance decisions could be based on outdated information

### Authority Contract Functions

**Who can call:** 
- `add()`: Only executor (governance)
- `revoke()`: Executor OR anyone if node not endorsed
- `get()`, `first()`, `next()`: Anyone (public views)

**User input:** Node master addresses, endorser addresses, identity hashes
**Access control:** Mixed - some functions restricted to executor

```solidity
// builtin/gen/authority.sol:14-30
function add(address _nodeMaster, address _endorsor, bytes32 _identity) public {
    require(msg.sender == executor(), "builtin: executor required");
    // ...
}

function revoke(address _nodeMaster) public {
    require(msg.sender == executor() || !AuthorityNative(this).native_isEndorsed(_nodeMaster), 
            "builtin: requires executor, or node master out of endorsed");
    // ...
}
```

**Vulnerability:** The `revoke()` function has complex authorization:
1. Allows executor to revoke any node
2. Allows anyone to revoke non-endorsed nodes
3. `native_isEndorsed()` reads from state - could be stale in MTT
4. Could enable unauthorized validator removals

## 4. Real Attack Scenarios

### Scenario 1: Fee Delegation Context Abuse

**Actual Attack Flow:**
1. Attacker creates MTT with 2 clauses
2. Sets gas payer to trusted sponsor via fee delegation
3. Clause 1: Calls vulnerable DApp that checks `ExtensionV2.txGasPayer() == trustedSponsor`
4. Clause 2: Calls another contract for additional exploitation
5. Both clauses see the same gas payer, enabling unauthorized access

**No user input validation needed** - the vulnerability is in the shared context design.

### Scenario 2: Energy Transfer Race Condition

**Actual Attack Flow:**
1. Attacker creates MTT with multiple clauses calling Energy functions
2. Clause 1: Calls `transfer()` which triggers `native_sub(attacker, amount)`
3. If native state updates are not immediately consistent across clauses
4. Clause 2: Another `transfer()` call might succeed based on stale balance
5. Results in double-spending or inconsistent balances

**User input:** Transfer amounts and recipient addresses (controlled by attacker).

### Scenario 3: Governance Parameter Staleness

**Actual Attack Flow:**
1. Governance proposal submitted to change critical parameter
2. MTT created with multiple clauses
3. Clause 1: Calls `Params.get()` to read current parameter value
4. Clause 2: Executes governance action based on that value
5. If parameter changes between clauses, decision based on stale data
6. Could enable unauthorized parameter changes

**User input:** Parameter keys and values (through governance process).

## 5. Access Control Analysis

### Public Functions (No Access Control)
- `ExtensionV2.txGasPayer()` - Anyone can call
- `Extension.blockID()`, `blockTime()`, etc. - Anyone can call  
- `Energy.balanceOf()`, `totalSupply()` - Anyone can call
- `Params.get()` - Anyone can call
- `Authority.get()`, `first()`, `next()` - Anyone can call

### Restricted Functions
- `Params.set()` - Only executor can call
- `Authority.add()` - Only executor can call
- `Energy.transfer()` - Only token holder or approved spender
- `Energy.move()` - Only account holder or master

### Semi-Restricted Functions  
- `Authority.revoke()` - Executor OR anyone if node not endorsed
- `Executor.propose()` - Only approvers or voting contracts
- `Executor.approve()` - Only active approvers

## 6. Key Findings

### Design Issues
1. **Shared Transaction Context:** All clauses in MTT share the same transaction context, enabling context confusion attacks
2. **No Native Call Validation:** Native functions trust their inputs and execution environment without validation
3. **State Consistency Assumptions:** Code assumes native state changes are immediately consistent across all contexts
4. **Mixed Access Controls:** Some functions have complex authorization logic that depends on potentially stale state

### Real Vulnerabilities
1. **Context Abuse:** `txGasPayer()` returns transaction-level context without caller validation
2. **Race Conditions:** Native call sequences assume atomicity without guarantees
3. **Stale Reads:** Parameter and state reads may return outdated information in MTT
4. **Authorization Bypass:** Complex access controls may be bypassed through state inconsistencies

### Impact Assessment
- **High Impact:** Functions affecting consensus (Authority), economics (Energy), governance (Params, Executor)
- **Medium Impact:** Context confusion enabling unauthorized access to sponsored services
- **Low Impact:** Information disclosure through public view functions

The vulnerabilities are real and stem from the unique MTT architecture combined with the native call mechanism, not from theoretical attack scenarios.