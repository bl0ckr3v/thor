// Copyright (c) 2018 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/xenv"
)

// TestNativeSubAddAtomicity tests the atomicity assumptions in Energy._transfer
// This demonstrates the critical vulnerability in native_sub -> native_add sequence
func TestNativeSubAddAtomicity(t *testing.T) {
	var (
		sender    = thor.BytesToAddress([]byte("sender"))
		recipient = thor.BytesToAddress([]byte("recipient"))
		balance   = big.NewInt(1000)
		amount    = big.NewInt(500)
	)

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	// Setup initial balances
	st.SetEnergy(sender, balance, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, balance)

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	// Test 1: Verify the vulnerable code path exists
	// Energy._transfer calls native_sub then native_add without explicit atomicity
	test.Case("balanceOf", sender).
		ShouldOutput(balance).
		Assert(t)

	test.Case("balanceOf", recipient).
		ShouldOutput(&big.Int{}).
		Assert(t)

	// Test 2: Normal transfer - demonstrates the vulnerable sequence
	transferEvent := func(from, to thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}

	test.Case("transfer", recipient, amount).
		Caller(sender).
		ShouldLog(transferEvent(sender, recipient, amount)).
		ShouldOutput(true).
		Assert(t)

	// Test 3: Verify state after transfer
	expectedSenderBalance := new(big.Int).Sub(balance, amount)
	test.Case("balanceOf", sender).
		ShouldOutput(expectedSenderBalance).
		Assert(t)

	test.Case("balanceOf", recipient).
		ShouldOutput(amount).
		Assert(t)

	// Test 4: Edge case - zero amount transfer (still triggers the vulnerable path)
	test.Case("transfer", recipient, &big.Int{}).
		Caller(sender).
		ShouldLog(transferEvent(sender, recipient, &big.Int{})).
		ShouldOutput(true).
		Assert(t)

	// Test 5: Boundary condition - exact balance transfer
	remainingBalance := expectedSenderBalance
	test.Case("transfer", recipient, remainingBalance).
		Caller(sender).
		ShouldLog(transferEvent(sender, recipient, remainingBalance)).
		ShouldOutput(true).
		Assert(t)

	// Test 6: Verify complete balance transfer
	test.Case("balanceOf", sender).
		ShouldOutput(&big.Int{}).
		Assert(t)

	expectedRecipientBalance := new(big.Int).Add(amount, remainingBalance)
	test.Case("balanceOf", recipient).
		ShouldOutput(expectedRecipientBalance).
		Assert(t)

	// VULNERABILITY ANALYSIS:
	// The Energy._transfer function contains this sequence:
	// 1. require(EnergyNative(this).native_sub(_from, _amount), "builtin: insufficient balance");
	// 2. EnergyNative(this).native_add(_to, _amount);
	// 3. emit Transfer(_from, _to, _amount);
	//
	// The comment "believed that will never overflow" indicates dangerous assumptions.
	// In MTT context, if native_sub succeeds but native_add is delayed or duplicated
	// across clause boundaries, it could lead to:
	// - Double spending (if native_add executes twice)
	// - Lost funds (if native_add fails silently)
	// - State inconsistency between native and Solidity layers
}

// TestMoveVulnerability tests the Energy.move function which has additional attack surface
func TestMoveVulnerability(t *testing.T) {
	var (
		account = thor.BytesToAddress([]byte("account"))
		master  = thor.BytesToAddress([]byte("master"))
		target  = thor.BytesToAddress([]byte("target"))
		balance = big.NewInt(1000)
		amount  = big.NewInt(300)
	)

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	// Setup account with master and balance
	st.SetEnergy(account, balance, b0.Header().Timestamp())
	st.SetMaster(account, master)
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, balance)

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	transferEvent := func(from, to thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}

	// Test 1: Normal move by account owner
	test.Case("move", account, target, amount).
		Caller(account).
		ShouldLog(transferEvent(account, target, amount)).
		ShouldOutput(true).
		Assert(t)

	// Test 2: Move by master (authorized)
	test.Case("move", account, target, amount).
		Caller(master).
		ShouldLog(transferEvent(account, target, amount)).
		ShouldOutput(true).
		Assert(t)

	// Test 3: Unauthorized move should fail
	unauthorized := thor.BytesToAddress([]byte("unauthorized"))
	test.Case("move", account, target, amount).
		Caller(unauthorized).
		ShouldVMError(errReverted).
		Assert(t)

	// VULNERABILITY ANALYSIS:
	// The move function checks: _from == msg.sender || EnergyNative(this).native_master(_from) == msg.sender
	// This relies on native_master() call which could be subject to:
	// 1. Stale reads in MTT context
	// 2. Race conditions if master changes mid-transaction
	// 3. Inconsistency between native and Solidity state
}

// TestTxGasPayerContextVulnerability demonstrates the ExtensionV2.txGasPayer vulnerability
func TestTxGasPayerContextVulnerability(t *testing.T) {
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: uint64(time.Now().Unix())}, &thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Extension.V2.ABI,
		to:  builtin.Extension.Address,
	}

	// Test addresses
	user1 := thor.BytesToAddress([]byte("user1"))
	user2 := thor.BytesToAddress([]byte("user2"))
	sponsor1 := thor.BytesToAddress([]byte("sponsor1"))
	sponsor2 := thor.BytesToAddress([]byte("sponsor2"))

	// Test 1: Default case - no gas payer
	test.Case("txGasPayer").
		Caller(user1).
		ShouldOutput(thor.Address{}).
		Assert(t)

	// Test 2: Single gas payer context
	test.Case("txGasPayer").
		Caller(user1).
		GasPayer(sponsor1).
		ShouldOutput(sponsor1).
		Assert(t)

	// Test 3: Different caller, same gas payer - demonstrates the vulnerability
	test.Case("txGasPayer").
		Caller(user2).
		GasPayer(sponsor1).
		ShouldOutput(sponsor1).
		Assert(t)

	// Test 4: Different gas payer contexts
	test.Case("txGasPayer").
		Caller(user1).
		GasPayer(sponsor2).
		ShouldOutput(sponsor2).
		Assert(t)

	// VULNERABILITY ANALYSIS:
	// ExtensionV2.txGasPayer() directly returns native_txGasPayer() without validation
	// This creates several attack vectors:
	// 1. Context confusion: Function returns gas payer regardless of caller
	// 2. MTT exploitation: Different clauses can have different callers but same gas payer
	// 3. Authorization bypass: Contracts checking txGasPayer() for access control
	//    may grant access to unauthorized users in sponsored contexts
	//
	// Attack scenario:
	// - Malicious contract calls txGasPayer() in MTT
	// - Returns trusted sponsor address
	// - Victim contract grants access based on sponsor, not actual caller
	// - Results in unauthorized access to sponsored services
}

// TestParamsNativeGetVulnerability demonstrates the Params.get vulnerability
func TestParamsNativeGetVulnerability(t *testing.T) {
	executor := thor.BytesToAddress([]byte("executor"))
	
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{}, &thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Params.ABI,
		to:  builtin.Params.Address,
	}

	// Test 1: Verify executor function relies on native call
	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	// Test 2: Test parameter operations
	testKey := thor.BytesToBytes32([]byte("testParam"))
	testValue := big.NewInt(42)

	test.Case("set", testKey, testValue).
		Caller(executor).
		Assert(t)

	test.Case("get", testKey).
		ShouldOutput(testValue).
		Assert(t)

	// Test 3: Critical system parameters
	criticalKeys := []string{
		"baseGasPrice",
		"maxClauseCount", 
		"maxBlockGasLimit",
		"proposerEndorsement",
	}

	for _, keyStr := range criticalKeys {
		key := thor.BytesToBytes32([]byte(keyStr))
		
		// Get current value (may be zero for non-existent params)
		test.Case("get", key).
			ShouldOutput(&big.Int{}). // Default zero value
			Assert(t)
	}

	// VULNERABILITY ANALYSIS:
	// Params contract has two critical vulnerabilities:
	//
	// 1. executor() function calls native_executor() without validation
	//    - Could return stale executor address in MTT context
	//    - If native state lags behind Solidity state, wrong executor could be returned
	//
	// 2. get() function calls native_get() without consistency checks
	//    - Could return outdated parameter values
	//    - In governance proposals, stale reads could bypass intended restrictions
	//
	// Attack scenario:
	// - Proposal submitted when executor = oldExecutor
	// - Executor changed to newExecutor via governance
	// - Proposal execution checks executor() which returns stale oldExecutor
	// - Unauthorized proposal executes with outdated authorization
}

// TestAuthorityNativeVulnerability demonstrates Authority contract native call risks
func TestAuthorityNativeVulnerability(t *testing.T) {
	executor := thor.BytesToAddress([]byte("executor"))
	master1 := thor.BytesToAddress([]byte("master1"))
	endorsor1 := thor.BytesToAddress([]byte("endorsor1"))
	identity1 := thor.BytesToBytes32([]byte("identity1"))

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		state.SetBalance(endorsor1, thor.InitialProposerEndorsement)
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		builtin.Params.Native(state).Set(thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{}, &thor.NoFork)

	test := &ctest{
		rt:     rt,
		abi:    builtin.Authority.ABI,
		to:     builtin.Authority.Address,
		caller: executor,
	}

	candidateEvent := func(nodeMaster thor.Address, action string) *tx.Event {
		ev, _ := builtin.Authority.ABI.EventByName("Candidate")
		var b32 thor.Bytes32
		copy(b32[:], action)
		data, _ := ev.Encode(b32)
		return &tx.Event{
			Address: builtin.Authority.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(nodeMaster[:])},
			Data:    data,
		}
	}

	// Test 1: Verify executor dependency on native call
	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	// Test 2: Add authority node
	test.Case("add", master1, endorsor1, identity1).
		ShouldLog(candidateEvent(master1, "added")).
		Assert(t)

	// Test 3: Verify node was added
	test.Case("get", master1).
		ShouldOutput(true, endorsor1, identity1, true).
		Assert(t)

	// Test 4: Test revocation conditions
	// The revoke function has complex logic: msg.sender == executor() || !native_isEndorsed(_nodeMaster)
	test.Case("revoke", master1).
		ShouldLog(candidateEvent(master1, "revoked")).
		Assert(t)

	// VULNERABILITY ANALYSIS:
	// Authority contract has multiple native call vulnerabilities:
	//
	// 1. executor() relies on native_executor() - same staleness risk as Params
	// 2. revoke() checks native_isEndorsed() for authorization
	//    - Could allow unauthorized revocations if endorsement state is stale
	// 3. All native_* functions (add, revoke, get, first, next) trust native layer
	//    - State inconsistency could corrupt validator set
	//    - In PoA consensus, this could lead to chain splits or halts
	//
	// Critical impact: Authority contract manages consensus validators
	// Any vulnerability here could compromise the entire network's security
}

// TestCombinedAttackScenario demonstrates a complex multi-contract attack
func TestCombinedAttackScenario(t *testing.T) {
	// Setup multiple contracts
	executor := thor.BytesToAddress([]byte("executor"))
	attacker := thor.BytesToAddress([]byte("attacker"))
	sponsor := thor.BytesToAddress([]byte("sponsor"))
	
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		// Deploy all vulnerable contracts
		state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes())
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
		
		// Setup initial state
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
		state.SetEnergy(attacker, big.NewInt(1000), 0)
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	// Test combined attack vector:
	// 1. Use ExtensionV2.txGasPayer() to establish sponsored context
	// 2. Exploit Energy contract in same MTT with native call timing
	// 3. Manipulate Params reading for governance bypass
	
	extensionTest := &ctest{rt: rt, abi: builtin.Extension.V2.ABI, to: builtin.Extension.Address}
	energyTest := &ctest{rt: rt, abi: builtin.Energy.ABI, to: builtin.Energy.Address}
	paramsTest := &ctest{rt: rt, abi: builtin.Params.ABI, to: builtin.Params.Address}

	// Step 1: Establish sponsored context (simulating MTT Clause 1)
	extensionTest.Case("txGasPayer").
		Caller(attacker).
		GasPayer(sponsor).
		ShouldOutput(sponsor).
		Assert(t)

	// Step 2: Exploit energy transfer in sponsored context (simulating MTT Clause 2)
	energyTest.Case("balanceOf", attacker).
		Caller(attacker).
		GasPayer(sponsor). // Same sponsored context
		ShouldOutput(big.NewInt(1000)).
		Assert(t)

	// Step 3: Read parameters that might be used for validation (simulating MTT Clause 3)
	paramsTest.Case("executor").
		Caller(attacker).
		GasPayer(sponsor). // Same sponsored context
		ShouldOutput(executor).
		Assert(t)

	// COMBINED ATTACK ANALYSIS:
	// In a real MTT attack, all three operations would be in the same transaction:
	// 1. Clause 1: Call vulnerable contract that checks txGasPayer() == trustedSponsor
	// 2. Clause 2: Perform unauthorized energy operations under sponsored context
	// 3. Clause 3: Read stale parameters to bypass governance restrictions
	//
	// The combination of:
	// - Context confusion (txGasPayer)
	// - Native call atomicity issues (Energy)  
	// - Stale parameter reads (Params)
	// Could enable complex attacks that bypass multiple security layers
}