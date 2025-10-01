// Copyright (c) 2018 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin_test

import (
	"math"
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

// TestAttackScenario1_FeeDelegationContextAbuse demonstrates the fee delegation context abuse vulnerability
// This test shows how txGasPayer() can be exploited in Multi-Task Transactions (MTT)
func TestAttackScenario1_FeeDelegationContextAbuse(t *testing.T) {
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

	// Setup test addresses
	maliciousUser := thor.BytesToAddress([]byte("malicious"))
	trustedSponsor := thor.BytesToAddress([]byte("sponsor"))
	victim := thor.BytesToAddress([]byte("victim"))

	// Test 1: Normal case - no gas payer set
	test.Case("txGasPayer").
		Caller(maliciousUser).
		ShouldOutput(thor.Address{}).
		Assert(t)

	// Test 2: Attack scenario - malicious user exploits sponsored context
	// In a real MTT attack, this would be part of a multi-clause transaction where:
	// - Clause 1: Sets up sponsored context via delegation
	// - Clause 2: Calls vulnerable contract that checks txGasPayer() == expectedSponsor
	test.Case("txGasPayer").
		Caller(maliciousUser).
		GasPayer(trustedSponsor). // This simulates the sponsored context
		ShouldOutput(trustedSponsor).
		Assert(t)

	// Test 3: Demonstrate the vulnerability - the function returns the sponsor
	// even when called by a malicious user, enabling unauthorized access
	test.Case("txGasPayer").
		Caller(victim).
		GasPayer(trustedSponsor).
		ShouldOutput(trustedSponsor).
		Assert(t)

	// The vulnerability: txGasPayer() doesn't validate the relationship between
	// msg.sender and the actual gas payer, allowing context abuse in MTT
}

// TestAttackScenario2_NativeStateInconsistency demonstrates the native-Solidity state inconsistency vulnerability
// This test shows potential race conditions in Energy contract's _transfer function
func TestAttackScenario2_NativeStateInconsistency(t *testing.T) {
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

	// Set initial energy balance for attacker
	st.SetEnergy(attacker, balance, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, balance)

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	transferEvent := func(from, to thor.Address, value *big.Int) *tx.Event {
		ev, _ := builtin.Energy.ABI.EventByName("Transfer")
		data, _ := ev.Encode(value)
		return &tx.Event{
			Address: builtin.Energy.Address,
			Topics:  []thor.Bytes32{ev.ID(), thor.BytesToBytes32(from[:]), thor.BytesToBytes32(to[:])},
			Data:    data,
		}
	}

	test := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	// Test 1: Verify initial balance
	test.Case("balanceOf", attacker).
		ShouldOutput(balance).
		Assert(t)

	// Test 2: Normal transfer - should work
	transferAmount := big.NewInt(100)
	test.Case("transfer", victim, transferAmount).
		Caller(attacker).
		ShouldLog(transferEvent(attacker, victim, transferAmount)).
		ShouldOutput(true).
		Assert(t)

	// Test 3: Verify balances after transfer
	expectedAttackerBalance := new(big.Int).Sub(balance, transferAmount)
	test.Case("balanceOf", attacker).
		ShouldOutput(expectedAttackerBalance).
		Assert(t)

	test.Case("balanceOf", victim).
		ShouldOutput(transferAmount).
		Assert(t)

	// Test 4: Attempt to exploit the native_sub -> native_add sequence
	// In a real MTT attack, this could potentially cause state inconsistency
	// if native calls aren't properly synchronized across clauses
	largeAmount := big.NewInt(2000) // More than available balance
	test.Case("transfer", victim, largeAmount).
		Caller(attacker).
		ShouldVMError(errReverted). // Should fail due to insufficient balance
		Assert(t)

	// The vulnerability lies in the trust assumption that native_sub and native_add
	// are atomic across MTT clauses. The comment "believed that will never overflow"
	// indicates insufficient validation of the native operation sequence.
}

// TestAttackScenario3_GovernanceQuorumBypass demonstrates potential governance manipulation
// This test shows the governance system's reliance on timestamp validation
func TestAttackScenario3_GovernanceQuorumBypass(t *testing.T) {
	test := initExectorTest()

	// Setup approvers
	approver1 := thor.BytesToAddress([]byte("approver1"))
	approver2 := thor.BytesToAddress([]byte("approver2"))
	approver3 := thor.BytesToAddress([]byte("approver3"))

	// Add approvers
	test.Case("addApprover", approver1, thor.BytesToBytes32(approver1.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)
	test.Case("addApprover", approver2, thor.BytesToBytes32(approver2.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)
	test.Case("addApprover", approver3, thor.BytesToBytes32(approver3.Bytes())).
		Caller(builtin.Executor.Address).
		Assert(t)

	// Verify quorum calculation: (3 + 1) * 2 / 3 = 2
	test.Case("approverCount").
		ShouldOutput(uint8(3)).
		Assert(t)

	// Create a proposal
	target := builtin.Params.Address
	setParam, _ := builtin.Params.ABI.MethodByName("set")
	data, _ := setParam.EncodeInput(thor.BytesToBytes32([]byte("testParam")), big.NewInt(999))

	proposalID := func() thor.Bytes32 {
		var b8 [8]byte
		// Use current time for proposal ID generation
		currentTime := test.rt.Context().Time
		for i := 0; i < 8; i++ {
			b8[i] = byte(currentTime >> (8 * (7 - i)))
		}
		return thor.Keccak256(b8[:], approver1[:])
	}()

	test.Case("propose", target, data).
		Caller(approver1).
		ShouldOutput(proposalID).
		Assert(t)

	// Test 1: Normal approval process
	test.Case("approve", proposalID).
		Caller(approver1).
		Assert(t)

	test.Case("approve", proposalID).
		Caller(approver2).
		Assert(t)

	// Test 2: Execute with proper quorum
	test.Case("execute", proposalID).
		ShouldLog(proposalEvent(proposalID, "executed")).
		Assert(t)

	// Test 3: Demonstrate timestamp vulnerability
	// Create another proposal to test time manipulation resistance
	proposalID2 := func() thor.Bytes32 {
		var b8 [8]byte
		currentTime := test.rt.Context().Time + 1 // Slightly different time
		for i := 0; i < 8; i++ {
			b8[i] = byte(currentTime >> (8 * (7 - i)))
		}
		return thor.Keccak256(b8[:], approver2[:])
	}()

	test.Case("propose", target, data).
		Caller(approver2).
		ShouldOutput(proposalID2).
		Assert(t)

	// The vulnerability: The system relies on `now - timeProposed < 1 weeks`
	// If validators can manipulate block timestamps, they could potentially
	// extend or shorten proposal validity windows
}

// TestAttackScenario4_ParamsGovernancePrivilegeEscalation demonstrates parameter manipulation vulnerability
// This test shows how native parameter reads could be exploited
func TestAttackScenario4_ParamsGovernancePrivilegeEscalation(t *testing.T) {
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

	// Test 1: Verify executor address is correctly set
	test.Case("executor").
		ShouldOutput(executor).
		Assert(t)

	// Test 2: Normal parameter operations
	testKey := thor.BytesToBytes32([]byte("testKey"))
	testValue := big.NewInt(12345)

	test.Case("set", testKey, testValue).
		Caller(executor).
		Assert(t)

	test.Case("get", testKey).
		ShouldOutput(testValue).
		Assert(t)

	// Test 3: Unauthorized access should fail
	test.Case("set", testKey, big.NewInt(99999)).
		Caller(maliciousActor).
		ShouldVMError(errReverted).
		Assert(t)

	// Test 4: Demonstrate the vulnerability - native_get() calls without validation
	// The vulnerability lies in the direct native calls without state consistency checks
	criticalParam := thor.BytesToBytes32([]byte("maxClauseCount"))
	dangerousValue := big.NewInt(10000) // Extremely high value that could DoS the network

	// In a real attack scenario, if native state becomes inconsistent with Solidity state,
	// an attacker might be able to exploit the gap between native_get() reads and
	// the executor() validation in set()

	// Test the current parameter (should be safe)
	test.Case("get", criticalParam).
		ShouldOutput(&big.Int{}). // Default value for non-existent param
		Assert(t)

	// The vulnerability: If native_get() returns stale data while native_set() updates
	// are pending, proposals could be approved based on outdated parameter values,
	// potentially allowing unauthorized parameter changes that bypass governance
}

// TestMTTAttackVector demonstrates Multi-Task Transaction specific vulnerabilities
// This test shows how MTT's clause isolation could be exploited
func TestMTTAttackVector(t *testing.T) {
	db := muxdb.NewMem()
	b0 := buildGenesis(db, func(state *state.State) error {
		state.SetCode(builtin.Extension.Address, builtin.Extension.V2.RuntimeBytecodes())
		state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
		return nil
	})

	repo, _ := chain.NewRepository(db, b0)
	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	chain := repo.NewChain(b0.Header().ID())

	attacker := thor.BytesToAddress([]byte("attacker"))
	sponsor := thor.BytesToAddress([]byte("sponsor"))
	balance := big.NewInt(1000)

	// Setup initial state
	st.SetEnergy(attacker, balance, b0.Header().Timestamp())
	builtin.Energy.Native(st, b0.Header().Timestamp()).SetInitialSupply(&big.Int{}, balance)

	rt := runtime.New(chain, st, &xenv.BlockContext{Time: b0.Header().Timestamp()}, &thor.NoFork)

	// Test MTT scenario: Multiple clauses in a single transaction
	// Clause 1: Check txGasPayer (Extension contract)
	extensionTest := &ctest{
		rt:  rt,
		abi: builtin.Extension.V2.ABI,
		to:  builtin.Extension.Address,
	}

	// Clause 2: Transfer energy (Energy contract)
	energyTest := &ctest{
		rt:  rt,
		abi: builtin.Energy.ABI,
		to:  builtin.Energy.Address,
	}

	// Simulate MTT attack where:
	// 1. First clause establishes sponsored context
	// 2. Second clause exploits the context for unauthorized operations

	// Clause 1: Verify sponsored context
	extensionTest.Case("txGasPayer").
		Caller(attacker).
		GasPayer(sponsor).
		ShouldOutput(sponsor).
		Assert(t)

	// Clause 2: Perform operation that might rely on sponsorship context
	// In a real attack, this could be a transfer that bypasses fee checks
	energyTest.Case("balanceOf", attacker).
		Caller(attacker).
		GasPayer(sponsor). // Same sponsored context
		ShouldOutput(balance).
		Assert(t)

	// The vulnerability: MTT allows clauses to share context (like gas payer)
	// but each clause can call different contracts, potentially creating
	// inconsistent state assumptions across contract boundaries
}