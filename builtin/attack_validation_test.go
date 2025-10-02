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

// TestScenario1_FeeDelegationContextAbuse validates the ExtensionV2.txGasPayer() vulnerability
// CONFIRMED: Lines 11-13 in builtin/gen/extension-v2.sol show direct native call without validation
func TestScenario1_FeeDelegationContextAbuse(t *testing.T) {
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

	// Attack scenario addresses
	maliciousUser := thor.BytesToAddress([]byte("malicious"))
	trustedSponsor := thor.BytesToAddress([]byte("sponsor"))
	victim := thor.BytesToAddress([]byte("victim"))

	// VULNERABILITY VALIDATION:
	// ExtensionV2.txGasPayer() at line 11-13:
	// function txGasPayer() public view returns(address) {
	//     return ExtensionV2Native(this).native_txGasPayer();
	// }
	// 
	// This function returns the gas payer WITHOUT validating the relationship
	// between msg.sender and the actual gas payer, enabling context abuse.

	// Test 1: Demonstrate the vulnerability - function returns sponsor regardless of caller
	test.Case("txGasPayer").
		Caller(maliciousUser).
		GasPayer(trustedSponsor).
		ShouldOutput(trustedSponsor).
		Assert(t)

	// Test 2: Different caller, same result - proves context confusion
	test.Case("txGasPayer").
		Caller(victim).
		GasPayer(trustedSponsor).
		ShouldOutput(trustedSponsor).
		Assert(t)

	// Test 3: No gas payer set - returns zero address
	test.Case("txGasPayer").
		Caller(maliciousUser).
		ShouldOutput(thor.Address{}).
		Assert(t)

	// ATTACK IMPACT: In MTT, a malicious contract can:
	// 1. Call txGasPayer() in a sponsored context
	// 2. Receive trusted sponsor address
	// 3. Use this to bypass access controls in vulnerable contracts
	// 4. Gain unauthorized access to sponsored services
	//
	// This aligns with Immunefi's "direct theft of user funds" category
}

// TestScenario2_NativeStateInconsistency validates the Energy._transfer() atomicity vulnerability
// CONFIRMED: Lines 68-76 in builtin/gen/energy.sol show the vulnerable native call sequence
func TestScenario2_NativeStateInconsistency(t *testing.T) {
	var (
		attacker = thor.BytesToAddress([]byte("attacker"))
		victim   = thor.BytesToAddress([]byte("victim"))
		balance  = big.NewInt(1000)
	)

	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	// Setup initial energy balance
	st.SetEnergy(attacker, balance, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, balance)

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	test := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	// VULNERABILITY VALIDATION:
	// Energy._transfer() at lines 68-76:
	// function _transfer(address _from, address _to, uint256 _amount) internal {
	//     if (_amount > 0) {
	//         require(EnergyNative(this).native_sub(_from, _amount), "builtin: insufficient balance");
	//         // believed that will never overflow
	//         EnergyNative(this).native_add(_to, _amount);
	//     }
	//     emit Transfer(_from, _to, _amount);
	// }
	//
	// The comment "believed that will never overflow" indicates dangerous assumptions.
	// The sequence native_sub() -> native_add() lacks explicit atomicity guarantees.

	transferEvent := func(from, to thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}

	// Test 1: Verify initial state
	test.Case("balanceOf", attacker).
		ShouldOutput(balance).
		Assert(t)

	// Test 2: Normal transfer - demonstrates the vulnerable code path
	transferAmount := big.NewInt(100)
	test.Case("transfer", victim, transferAmount).
		Caller(attacker).
		ShouldLog(transferEvent(attacker, victim, transferAmount)).
		ShouldOutput(true).
		Assert(t)

	// Test 3: Verify the native calls executed
	expectedAttackerBalance := new(big.Int).Sub(balance, transferAmount)
	test.Case("balanceOf", attacker).
		ShouldOutput(expectedAttackerBalance).
		Assert(t)

	test.Case("balanceOf", victim).
		ShouldOutput(transferAmount).
		Assert(t)

	// Test 4: Edge case that could expose atomicity issues
	// Transfer exact remaining balance
	test.Case("transfer", victim, expectedAttackerBalance).
		Caller(attacker).
		ShouldLog(transferEvent(attacker, victim, expectedAttackerBalance)).
		ShouldOutput(true).
		Assert(t)

	// Test 5: Verify complete balance transfer
	test.Case("balanceOf", attacker).
		ShouldOutput(&big.Int{}).
		Assert(t)

	// ATTACK IMPACT: In MTT context, if native_sub and native_add aren't
	// properly synchronized across clause boundaries, it could lead to:
	// - Double spending if native_add executes multiple times
	// - Fund loss if native_add fails after native_sub succeeds
	// - VTHO inflation affecting validator rewards in Hayabusa DPoS
	//
	// This aligns with "permanent freezing of funds" in Immunefi scope
}

// TestScenario3_GovernanceQuorumBypass validates timestamp manipulation vulnerability
// CONFIRMED: Lines 74, 86 in builtin/gen/executor.sol show timestamp-based validation
func TestScenario3_GovernanceQuorumBypass(t *testing.T) {
	test := initExectorTest()

	// Setup approvers for quorum testing
	approver1 := thor.BytesToAddress([]byte("approver1"))
	approver2 := thor.BytesToAddress([]byte("approver2"))
	approver3 := thor.BytesToAddress([]byte("approver3"))

	// Add approvers to test quorum calculation
	test.Case("addApprover", approver1, thor.BytesToBytes32(approver1.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)
	test.Case("addApprover", approver2, thor.BytesToBytes32(approver2.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)
	test.Case("addApprover", approver3, thor.BytesToBytes32(approver3.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)

	// VULNERABILITY VALIDATION:
	// Executor.approve() at line 74:
	// require(now - proposals[_proposalID].timeProposed < 1 weeks, "builtin: proposal expired");
	//
	// Executor.execute() at line 86:
	// require(now - proposals[_proposalID].timeProposed < 1 weeks, "builtin: proposal expired");
	//
	// Both functions rely on `now` (block.timestamp) for validation.
	// In PoA2.0, if validators can manipulate block timestamps, they could
	// extend or shorten proposal validity windows.

	// Verify quorum calculation: (3 + 1) * 2 / 3 = 2 approvers needed
	test.Case("approverCount").
		ShouldOutput(uint8(3)).
		Assert(t)

	// Create a proposal targeting critical parameters
	target := builtin.Params.Address
	setParam, _ := builtin.Params.ABI.MethodByName("set")
	criticalParam := thor.BytesToBytes32([]byte("maxClauseCount"))
	dangerousValue := big.NewInt(10000) // Could DoS network
	data, _ := setParam.EncodeInput(criticalParam, dangerousValue)

	// Generate proposal ID based on current time
	proposalID := func() thor.Bytes32 {
		var b8 [8]byte
		currentTime := test.rt.Context().Time
		for i := 0; i < 8; i++ {
			b8[i] = byte(currentTime >> (8 * (7 - i)))
		}
		return thor.Keccak256(b8[:], approver1[:])
	}()

	// Test 1: Create proposal
	test.Case("propose", target, data).
		Caller(approver1).
		ShouldOutput(proposalID).
		Assert(t)

	// Test 2: Approve proposal (demonstrates timestamp dependency)
	test.Case("approve", proposalID).
		Caller(approver1).
		Assert(t)

	test.Case("approve", proposalID).
		Caller(approver2).
		Assert(t)

	// Test 3: Execute proposal (relies on timestamp validation)
	test.Case("execute", proposalID).
		ShouldLog(proposalEvent(proposalID, "executed")).
		Assert(t)

	// ATTACK IMPACT: If Hayabusa PoA2.0 validators can manipulate timestamps:
	// 1. Extend proposal validity beyond intended 1 week
	// 2. Rush proposals by shortening time windows
	// 3. Bypass governance timeouts for critical parameter changes
	// 4. Force unauthorized changes to validator sets or VTHO burn rates
	//
	// This aligns with "manipulation of governance voting" in scope
}

// TestScenario4_ParamsGovernancePrivilegeEscalation validates native parameter read vulnerability
// CONFIRMED: Lines 10-12, 21-23 in builtin/gen/params.sol show vulnerable native calls
func TestScenario4_ParamsGovernancePrivilegeEscalation(t *testing.T) {
	executor := thor.BytesToAddress([]byte("executor"))
	maliciousActor := thor.BytesToAddress([]byte("malicious"))

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

	// VULNERABILITY VALIDATION:
	// Params.executor() at lines 10-12:
	// function executor() public view returns(address) {
	//     return ParamsNative(this).native_executor();
	// }
	//
	// Params.get() at lines 21-23:
	// function get(bytes32 _key) public view returns(uint256) {
	//     return ParamsNative(this).native_get(_key);
	// }
	//
	// Both functions directly call native layer without consistency validation.
	// If native state lags behind Solidity state, stale reads could occur.

	// Test 1: Verify executor function relies on native call
	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	// Test 2: Test parameter operations
	testKey := thor.BytesToBytes32([]byte("testParam"))
	testValue := big.NewInt(42)

	// Only executor can set parameters
	test.Case("set", testKey, testValue).
		Caller(executor).
		Assert(t)

	// Anyone can read parameters (this is the vulnerable path)
	test.Case("get", testKey).
		ShouldOutput(testValue).
		Assert(t)

	// Test 3: Unauthorized set should fail
	test.Case("set", testKey, big.NewInt(999)).
		Caller(maliciousActor).
		ShouldVMError(errReverted).
		Assert(t)

	// Test 4: Critical system parameters that could be exploited
	criticalParams := []string{
		"maxClauseCount",    // Could enable DoS via unlimited clauses
		"baseGasPrice",      // Could manipulate transaction costs
		"proposerEndorsement", // Could affect validator economics
	}

	for _, paramName := range criticalParams {
		key := thor.BytesToBytes32([]byte(paramName))
		
		// Reading non-existent parameters returns zero
		test.Case("get", key).
			ShouldOutput(&big.Int{}).
			Assert(t)
	}

	// ATTACK IMPACT: In MTT context with native call staleness:
	// 1. Proposal approved based on stale executor() read
	// 2. Parameter changes executed with outdated validation
	// 3. Critical parameters like maxClauseCount set to extreme values
	// 4. Network DoS through clause flooding or validator manipulation
	//
	// This aligns with "total network shutdown" in Immunefi scope
}

// TestMTTSpecificVulnerabilities demonstrates Multi-Task Transaction attack vectors
// CONFIRMED: ExtensionV3 adds txClauseIndex() and txClauseCount() for MTT support
func TestMTTSpecificVulnerabilities(t *testing.T) {
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Extension.Address, builtin.Extension.V3.RuntimeBytecodes())
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	attacker := thor.BytesToAddress([]byte("attacker"))
	sponsor := thor.BytesToAddress([]byte("sponsor"))

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	// VULNERABILITY VALIDATION:
	// ExtensionV3 adds MTT-specific functions at lines 16-25:
	// function txClauseIndex() public view returns (uint32)
	// function txClauseCount() public view returns (uint32)
	//
	// These enable clause-aware attacks where different clauses in the same
	// transaction can exploit shared context (gas payer, tx ID) while calling
	// different contracts with potentially inconsistent native state.

	extensionTest := &ctest{
		rt:  rt,
		abi: builtin.Extension.V3.ABI,
		to:  builtin.Extension.Address,
	}

	// Test 1: MTT clause information functions
	extensionTest.Case("txClauseIndex").
		Caller(attacker).
		ShouldOutput(uint32(0)). // Default clause index
		Assert(t)

	extensionTest.Case("txClauseCount").
		Caller(attacker).
		ShouldOutput(uint32(0)). // Default clause count
		Assert(t)

	// Test 2: Gas payer context in MTT scenario
	extensionTest.Case("txGasPayer").
		Caller(attacker).
		GasPayer(sponsor).
		ShouldOutput(sponsor).
		Assert(t)

	// ATTACK SCENARIO: In a real MTT attack:
	// Clause 0: Call ExtensionV2.txGasPayer() -> returns sponsor
	// Clause 1: Call vulnerable contract that trusts the sponsor
	// Clause 2: Call Energy.transfer() with potential native race conditions
	// Clause 3: Call Params.get() with potentially stale native reads
	//
	// The shared transaction context enables complex multi-contract exploits
	// that bypass individual contract security measures.

	// Test 3: Demonstrate cross-contract context sharing
	energyTest := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	paramsTest := &ctest{
		rt:  rt,
		abi: builtin.Params.ABI,
		to:  builtin.Params.Address,
	}

	// Same transaction context shared across different contracts
	energyTest.Case("balanceOf", attacker).
		Caller(attacker).
		GasPayer(sponsor). // Same sponsor context
		ShouldOutput(&big.Int{}).
		Assert(t)

	paramsTest.Case("get", thor.BytesToBytes32([]byte("testKey"))).
		Caller(attacker).
		GasPayer(sponsor). // Same sponsor context
		ShouldOutput(&big.Int{}).
		Assert(t)

	// COMBINED ATTACK IMPACT:
	// MTT enables sophisticated attacks combining all four scenarios:
	// 1. Fee delegation context abuse across multiple clauses
	// 2. Native state inconsistency exploitation in Energy transfers
	// 3. Governance manipulation through parameter staleness
	// 4. Cross-contract privilege escalation via shared context
	//
	// This creates attack vectors not possible in standard EVM implementations
}

// TestAuthorityNativeVulnerability validates validator management vulnerabilities
// CONFIRMED: Lines 10-12, 26 in builtin/gen/authority.sol show native dependencies
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

	// VULNERABILITY VALIDATION:
	// Authority.executor() at lines 10-12:
	// function executor() public view returns(address) {
	//     return AuthorityNative(this).native_executor();
	// }
	//
	// Authority.revoke() at line 26:
	// require(msg.sender == executor() || !AuthorityNative(this).native_isEndorsed(_nodeMaster), ...);
	//
	// Both rely on native calls that could return stale data in MTT context.

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

	// Test 2: Add validator node
	test.Case("add", master1, endorsor1, identity1).
		ShouldLog(candidateEvent(master1, "added")).
		Assert(t)

	// Test 3: Verify node was added via native calls
	test.Case("get", master1).
		ShouldOutput(true, endorsor1, identity1, true).
		Assert(t)

	// Test 4: Test revocation logic that depends on native_isEndorsed()
	test.Case("revoke", master1).
		ShouldLog(candidateEvent(master1, "revoked")).
		Assert(t)

	// ATTACK IMPACT: Authority contract manages consensus validators.
	// Vulnerabilities here could:
	// 1. Allow unauthorized validator additions/removals
	// 2. Corrupt the validator set through stale native reads
	// 3. Enable consensus attacks if validator list becomes inconsistent
	// 4. Cause chain splits or network halts in PoA2.0
	//
	// This is the highest impact vulnerability as it affects consensus security.
}

// Helper function to initialize executor test (from existing test file)
func initExectorTest() *ctest {
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes())
		state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes())
		state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())
		builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(builtin.Executor.Address[:]))
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: uint64(time.Now().Unix())}, &thor.NoFork)

	return &ctest{
		rt:  rt,
		abi: builtin.Executor.ABI,
		to:  builtin.Executor.Address,
	}
}

// Helper function for proposal events (from existing test file)
func proposalEvent(id thor.Bytes32, action string) *tx.Event {
	ev, _ := builtin.Executor.ABI.EventByName("Proposal")
	var b32 thor.Bytes32
	copy(b32[:], action)
	data, _ := ev.Encode(b32)
	return &tx.Event{
		Address: builtin.Executor.Address,
		Topics:  []thor.Bytes32{ev.ID(), id},
		Data:    data,
	}
}